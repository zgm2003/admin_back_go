package aichat

import (
	"context"
	"errors"
	"sync"
	"time"
	"unicode/utf8"

	infraai "admin_back_go/internal/infra/ai"
	infrarealtime "admin_back_go/internal/infra/realtime"
	"admin_back_go/internal/shared/enum"
)

const (
	maxDeliveryBytes     = 16 * 1024
	defaultDeliveryWait  = 50 * time.Millisecond
	deliveryCommandQueue = 64
)

var (
	ErrDeliveryCommitterNotConfigured = errors.New("AI reply delivery committer is not configured")
	ErrDeliveryDeltaInvalid           = errors.New("AI reply delivery delta is invalid")
)

type deliverySinkOptions struct {
	DeliveryContext context.Context
	Committer       DeliveryCommitter
	Publisher       infrarealtime.Publisher
	CommandID       uint64
	Owner           string
	Token           uint64
	ConversationID  int64
	UserID          int64
	RequestID       string
	MaxWait         time.Duration
	Now             func() time.Time
}

type deliveryCommandKind uint8

const (
	deliveryAccept deliveryCommandKind = iota + 1
	deliveryFlush
	deliveryStop
	deliveryClose
)

type deliveryCommand struct {
	kind  deliveryCommandKind
	ctx   context.Context
	delta string
	reply chan error
}

type deliverySink struct {
	options deliverySinkOptions
	queue   chan deliveryCommand
	done    chan struct{}

	closeOnce sync.Once
	closeDone chan struct{}
	closeErr  error

	errorMu     sync.RWMutex
	terminalErr error
}

func newDeliverySink(options deliverySinkOptions) *deliverySink {
	if options.DeliveryContext == nil {
		options.DeliveryContext = context.Background()
	}
	if options.MaxWait <= 0 {
		options.MaxWait = defaultDeliveryWait
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	sink := &deliverySink{
		options:   options,
		queue:     make(chan deliveryCommand, deliveryCommandQueue),
		done:      make(chan struct{}),
		closeDone: make(chan struct{}),
	}
	go sink.run()
	return sink
}

func (s *deliverySink) Emit(ctx context.Context, event infraai.Event) error {
	if s == nil || event.Type != "delta" || event.DeltaText == "" {
		return nil
	}
	return s.accept(ctx, event.DeltaText)
}

func (s *deliverySink) Accept(delta string) error {
	return s.accept(context.Background(), delta)
}

func (s *deliverySink) accept(ctx context.Context, delta string) error {
	if delta == "" {
		return nil
	}
	if !utf8.ValidString(delta) {
		return infraai.FatalEventSink(ErrDeliveryDeltaInvalid)
	}
	return s.request(ctx, deliveryCommand{kind: deliveryAccept, delta: delta})
}

func (s *deliverySink) Flush(ctx context.Context) error {
	return s.request(ctx, deliveryCommand{kind: deliveryFlush})
}

func (s *deliverySink) Stop(ctx context.Context) error {
	return s.request(ctx, deliveryCommand{kind: deliveryStop})
}

func (s *deliverySink) Close(ctx context.Context) error {
	if s == nil {
		return nil
	}
	s.closeOnce.Do(func() {
		s.closeErr = s.request(ctx, deliveryCommand{kind: deliveryClose})
		close(s.closeDone)
	})
	<-s.closeDone
	return s.closeErr
}

func (s *deliverySink) request(ctx context.Context, command deliveryCommand) error {
	if s == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	command.ctx = ctx
	command.reply = make(chan error, 1)
	select {
	case <-s.done:
		return s.loadTerminalError()
	case <-ctx.Done():
		return ctx.Err()
	case s.queue <- command:
	}
	select {
	case err := <-command.reply:
		return err
	case <-ctx.Done():
		return ctx.Err()
	case <-s.done:
		select {
		case err := <-command.reply:
			return err
		default:
			return s.loadTerminalError()
		}
	}
}

func (s *deliverySink) run() {
	defer close(s.done)
	var (
		buffer          string
		lastContext     = context.Background()
		timer           *time.Timer
		timerChannel    <-chan time.Time
		stopped         = deliveryStopped(s.options.DeliveryContext)
		publisherActive = s.options.Publisher != nil
		terminalErr     error
		deliveryDone    = s.options.DeliveryContext.Done()
	)

	stopTimer := func() {
		if timer == nil {
			return
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timerChannel = nil
	}
	startTimer := func() {
		if buffer == "" || stopped || terminalErr != nil {
			return
		}
		if timer == nil {
			timer = time.NewTimer(s.options.MaxWait)
		} else {
			timer.Reset(s.options.MaxWait)
		}
		timerChannel = timer.C
	}
	discard := func() {
		buffer = ""
		stopTimer()
	}
	setFatal := func(err error) error {
		if err == nil {
			return nil
		}
		terminalErr = infraai.FatalEventSink(err)
		s.storeTerminalError(terminalErr)
		discard()
		return terminalErr
	}
	flush := func(ctx context.Context) error {
		if terminalErr != nil {
			return terminalErr
		}
		if stopped || deliveryStopped(s.options.DeliveryContext) {
			stopped = true
			discard()
			return nil
		}
		if buffer == "" {
			stopTimer()
			return nil
		}
		if s.options.Committer == nil {
			return setFatal(ErrDeliveryCommitterNotConfigured)
		}
		stopTimer()
		delta := buffer
		buffer = ""
		if ctx == nil {
			ctx = context.Background()
		}
		sequence, committed, err := s.options.Committer.CommitDelivery(ctx, DeliveryCommit{
			CommandID: s.options.CommandID,
			Owner:     s.options.Owner,
			Token:     s.options.Token,
			Delta:     delta,
			Now:       s.options.Now(),
		})
		if err != nil {
			return setFatal(err)
		}
		if !committed {
			stopped = true
			discard()
			return nil
		}
		if !publisherActive {
			return nil
		}
		event, err := BuildDeltaEvent(DeltaPayload{
			ConversationID: s.options.ConversationID,
			RequestID:      s.options.RequestID,
			DeliverySeq:    sequence,
			Delta:          delta,
		})
		if err != nil {
			return setFatal(err)
		}
		if err := s.options.Publisher.Publish(ctx, infrarealtime.Publication{
			Platform: enum.PlatformAdmin,
			UserID:   s.options.UserID,
			Envelope: event,
		}); err != nil {
			publisherActive = false
		}
		return nil
	}
	appendDelta := func(ctx context.Context, delta string) error {
		if terminalErr != nil {
			return terminalErr
		}
		if stopped || deliveryStopped(s.options.DeliveryContext) {
			stopped = true
			discard()
			return nil
		}
		lastContext = ctx
		for delta != "" {
			remaining := maxDeliveryBytes - len(buffer)
			if len(delta) <= remaining {
				buffer += delta
				delta = ""
				if len(buffer) == maxDeliveryBytes {
					if err := flush(lastContext); err != nil {
						return err
					}
				}
				continue
			}
			cut := utf8SafePrefixBytes(delta, remaining)
			if cut == 0 {
				if err := flush(lastContext); err != nil {
					return err
				}
				continue
			}
			buffer += delta[:cut]
			delta = delta[cut:]
			if err := flush(lastContext); err != nil {
				return err
			}
		}
		if buffer != "" && timerChannel == nil {
			startTimer()
		}
		return nil
	}

	for {
		select {
		case <-deliveryDone:
			stopped = true
			discard()
			deliveryDone = nil
		case <-timerChannel:
			timerChannel = nil
			_ = flush(lastContext)
		case command := <-s.queue:
			if deliveryStopped(s.options.DeliveryContext) {
				stopped = true
				discard()
			}
			var err error
			switch command.kind {
			case deliveryAccept:
				err = appendDelta(command.ctx, command.delta)
			case deliveryFlush:
				err = flush(command.ctx)
			case deliveryStop:
				stopped = true
				discard()
				err = terminalErr
			case deliveryClose:
				if stopped {
					discard()
					err = terminalErr
				} else {
					err = flush(command.ctx)
				}
				command.reply <- err
				stopTimer()
				return
			default:
				err = setFatal(errors.New("unknown AI reply delivery command"))
			}
			command.reply <- err
		}
	}
}

func (s *deliverySink) storeTerminalError(err error) {
	s.errorMu.Lock()
	s.terminalErr = err
	s.errorMu.Unlock()
}

func (s *deliverySink) loadTerminalError() error {
	s.errorMu.RLock()
	defer s.errorMu.RUnlock()
	return s.terminalErr
}

func splitDeliveryUTF8(value string, maxBytes int) []string {
	if value == "" || maxBytes <= 0 || !utf8.ValidString(value) {
		return nil
	}
	parts := make([]string, 0, (len(value)/maxBytes)+1)
	for len(value) > maxBytes {
		cut := utf8SafePrefixBytes(value, maxBytes)
		if cut == 0 {
			return nil
		}
		parts = append(parts, value[:cut])
		value = value[cut:]
	}
	if value != "" {
		parts = append(parts, value)
	}
	return parts
}

func utf8SafePrefixBytes(value string, maxBytes int) int {
	if maxBytes <= 0 || value == "" {
		return 0
	}
	if len(value) <= maxBytes {
		return len(value)
	}
	cut := maxBytes
	for cut > 0 && !utf8.RuneStart(value[cut]) {
		cut--
	}
	return cut
}

var _ infraai.EventSink = (*deliverySink)(nil)
