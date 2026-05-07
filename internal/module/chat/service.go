package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/enum"
	platformrealtime "admin_back_go/internal/platform/realtime"
)

const defaultPageSize = 20

type Service struct {
	repository        Repository
	realtimePublisher platformrealtime.Publisher
	logger            *slog.Logger
	now               func() time.Time
}

type Option func(*Service)

func WithRealtimePublisher(publisher platformrealtime.Publisher) Option {
	return func(s *Service) { s.realtimePublisher = publisher }
}

func WithLogger(logger *slog.Logger) Option {
	return func(s *Service) {
		if logger != nil {
			s.logger = logger
		}
	}
}

func WithClock(now func() time.Time) Option {
	return func(s *Service) {
		if now != nil {
			s.now = now
		}
	}
}

func NewService(repository Repository, opts ...Option) *Service {
	s := &Service{repository: repository, logger: slog.Default(), now: time.Now}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *Service) ListConversations(ctx context.Context, identity Identity, query ConversationListQuery) (*ConversationListResponse, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	identity, appErr = normalizeIdentity(identity)
	if appErr != nil {
		return nil, appErr
	}
	query.UserID = identity.UserID
	query.CurrentPage, query.PageSize, appErr = normalizePage(query.CurrentPage, query.PageSize)
	if appErr != nil {
		return nil, appErr
	}
	rows, total, err := repo.ListConversations(ctx, query)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询会话失败", err)
	}
	ids := make([]int64, 0, len(rows))
	privateIDs := make([]int64, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
		if row.Type == ConversationTypePrivate {
			privateIDs = append(privateIDs, row.ID)
		}
	}
	unreads, err := repo.ConversationUnreadCounts(ctx, identity.UserID, ids)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询会话未读数失败", err)
	}
	peers, err := repo.PrivateConversationPeers(ctx, privateIDs, identity.UserID)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询私聊用户失败", err)
	}
	list := make([]ConversationItem, 0, len(rows))
	for _, row := range rows {
		item := conversationItemFromRow(row, unreads[row.ID])
		if row.Type == ConversationTypePrivate {
			if peer, ok := peers[row.ID]; ok {
				item.Name = peer.Username
				item.Avatar = peer.Avatar
			}
		}
		list = append(list, item)
	}
	return &ConversationListResponse{List: list, Page: Page{PageSize: query.PageSize, CurrentPage: query.CurrentPage, TotalPage: totalPage(total, query.PageSize), Total: total}}, nil
}

func (s *Service) ListContacts(ctx context.Context, identity Identity) (*ContactListResponse, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	identity, appErr = normalizeIdentity(identity)
	if appErr != nil {
		return nil, appErr
	}
	rows, err := repo.ListContacts(ctx, identity.UserID)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询联系人失败", err)
	}
	list := make([]ContactItem, 0, len(rows))
	for _, row := range rows {
		list = append(list, contactItemFromRow(row))
	}
	return &ContactListResponse{List: list}, nil
}

func (s *Service) AddContact(ctx context.Context, identity Identity, input ContactInput) *apperror.Error {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	identity, appErr = normalizeIdentity(identity)
	if appErr != nil {
		return appErr
	}
	if input.UserID <= 0 {
		return apperror.BadRequest("联系人ID无效")
	}
	if input.UserID == identity.UserID {
		return apperror.BadRequest("不能添加自己为联系人")
	}
	user, err := repo.FindActiveUser(ctx, input.UserID)
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "查询用户失败", err)
	}
	if user == nil {
		return apperror.NotFound("用户不存在")
	}
	if err := repo.WithTx(ctx, func(tx Repository) error {
		exists, err := tx.ContactExists(ctx, identity.UserID, input.UserID)
		if err != nil {
			return err
		}
		if exists {
			return errContactExists
		}
		return tx.CreatePendingContactPair(ctx, identity.UserID, input.UserID, s.now())
	}); err != nil {
		if errors.Is(err, errContactExists) {
			return apperror.BadRequest("联系人已存在或请求待处理中")
		}
		return apperror.Wrap(apperror.CodeInternal, 500, "添加联系人失败", err)
	}
	return nil
}

func (s *Service) ConfirmContact(ctx context.Context, identity Identity, input ContactInput) *apperror.Error {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	identity, appErr = normalizeIdentity(identity)
	if appErr != nil {
		return appErr
	}
	if input.UserID <= 0 {
		return apperror.BadRequest("联系人ID无效")
	}
	if input.UserID == identity.UserID {
		return apperror.BadRequest("不能确认自己的联系人请求")
	}
	if err := repo.WithTx(ctx, func(tx Repository) error {
		contact, err := tx.FindContact(ctx, identity.UserID, input.UserID)
		if err != nil {
			return err
		}
		if contact == nil {
			return errContactNotFound
		}
		if contact.Status != ContactStatusPending {
			return errContactAlreadyHandled
		}
		if contact.IsInitiator == enum.CommonYes {
			return errConfirmOutgoingContact
		}
		return tx.ConfirmContactPair(ctx, identity.UserID, input.UserID, s.now())
	}); err != nil {
		switch {
		case errors.Is(err, errContactNotFound):
			return apperror.NotFound("联系人请求不存在")
		case errors.Is(err, errContactAlreadyHandled):
			return apperror.BadRequest("该联系人请求已处理")
		case errors.Is(err, errConfirmOutgoingContact):
			return apperror.BadRequest("不能确认自己发起的请求")
		default:
			return apperror.Wrap(apperror.CodeInternal, 500, "确认联系人失败", err)
		}
	}
	return nil
}

func (s *Service) DeleteContact(ctx context.Context, identity Identity, input ContactInput) *apperror.Error {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	identity, appErr = normalizeIdentity(identity)
	if appErr != nil {
		return appErr
	}
	if input.UserID <= 0 {
		return apperror.BadRequest("联系人ID无效")
	}
	if input.UserID == identity.UserID {
		return apperror.BadRequest("不能删除自己")
	}
	if err := repo.WithTx(ctx, func(tx Repository) error {
		contact, err := tx.FindContact(ctx, identity.UserID, input.UserID)
		if err != nil {
			return err
		}
		if contact == nil {
			return errContactNotFound
		}
		now := s.now()
		if err := tx.SoftDeleteContactPair(ctx, identity.UserID, input.UserID, now); err != nil {
			return err
		}
		if contact.Status == ContactStatusConfirmed {
			return tx.ClosePrivateConversationForContactPair(ctx, identity.UserID, input.UserID, now)
		}
		return nil
	}); err != nil {
		if errors.Is(err, errContactNotFound) {
			return apperror.NotFound("联系人不存在")
		}
		return apperror.Wrap(apperror.CodeInternal, 500, "删除联系人失败", err)
	}
	return nil
}

func (s *Service) CreatePrivate(ctx context.Context, identity Identity, input CreatePrivateInput) (*CreatePrivateResponse, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	identity, appErr = normalizeIdentity(identity)
	if appErr != nil {
		return nil, appErr
	}
	if input.UserID <= 0 {
		return nil, apperror.BadRequest("联系人ID无效")
	}
	if input.UserID == identity.UserID {
		return nil, apperror.BadRequest("不能与自己创建私聊")
	}
	user, err := repo.FindActiveUser(ctx, input.UserID)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询用户失败", err)
	}
	if user == nil {
		return nil, apperror.NotFound("用户不存在")
	}
	confirmed, err := repo.IsConfirmedContact(ctx, identity.UserID, input.UserID)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询联系人关系失败", err)
	}
	if !confirmed {
		return nil, apperror.BadRequest("对方不是你的联系人，请先添加联系人")
	}

	var conversationID int64
	if err := repo.WithTx(ctx, func(tx Repository) error {
		if err := tx.LockConfirmedContactPair(ctx, identity.UserID, input.UserID); err != nil {
			return err
		}
		existing, err := tx.FindPrivateConversation(ctx, identity.UserID, input.UserID)
		if err != nil {
			return err
		}
		if existing != nil {
			conversationID = existing.ID
			return tx.RestoreParticipant(ctx, existing.ID, identity.UserID)
		}
		created, err := tx.CreatePrivateConversation(ctx, identity.UserID, input.UserID, s.now())
		if err != nil {
			return err
		}
		conversationID = created.ID
		return nil
	}); err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "创建私聊失败", err)
	}
	row, err := repo.FindConversationRow(ctx, conversationID, identity.UserID)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询私聊失败", err)
	}
	if row == nil {
		return nil, apperror.Internal("私聊会话未创建")
	}
	item := conversationItemFromRow(*row, 0)
	item.Name = user.Username
	item.Avatar = user.Avatar
	return &CreatePrivateResponse{Conversation: item}, nil
}

func (s *Service) SendMessage(ctx context.Context, identity Identity, input SendMessageInput) (*SendMessageResponse, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	identity, appErr = normalizeIdentity(identity)
	if appErr != nil {
		return nil, appErr
	}
	input, appErr = normalizeSendMessageInput(input)
	if appErr != nil {
		return nil, appErr
	}
	ok, err := repo.IsActiveParticipant(ctx, input.ConversationID, identity.UserID)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询会话权限失败", err)
	}
	if !ok {
		return nil, apperror.Forbidden("无权操作")
	}
	participants, err := repo.ActiveParticipantUserIDs(ctx, input.ConversationID)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询会话成员失败", err)
	}
	var metaJSON *string
	if input.MetaJSON != nil {
		raw, err := json.Marshal(input.MetaJSON)
		if err != nil {
			return nil, apperror.BadRequest("消息元数据格式错误")
		}
		value := string(raw)
		metaJSON = &value
	}
	now := s.now()
	message, err := repo.CreateMessage(ctx, CreateMessageInput{ConversationID: input.ConversationID, SenderID: identity.UserID, Type: input.Type, Content: input.Content, MetaJSON: metaJSON, CreatedAt: now})
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "发送消息失败", err)
	}
	preview := messagePreview(input.Type, input.Content)
	if err := repo.UpdateConversationLastMessage(ctx, input.ConversationID, message.ID, now, preview); err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "更新会话摘要失败", err)
	}
	sender, err := repo.FindUserBrief(ctx, identity.UserID)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询发送者失败", err)
	}
	item := messageItemFromMessage(*message, sender)
	s.publishToUsers(ctx, identity.Platform, participants, EventChatMessageCreatedV1, fmt.Sprintf("chat-message-%d", message.ID), map[string]any{"conversation_id": input.ConversationID, "message": item})
	return &SendMessageResponse{Message: item}, nil
}

func (s *Service) ListMessages(ctx context.Context, identity Identity, query MessageListQuery) (*MessageListResponse, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	identity, appErr = normalizeIdentity(identity)
	if appErr != nil {
		return nil, appErr
	}
	if query.ConversationID <= 0 {
		return nil, apperror.BadRequest("会话ID无效")
	}
	query.CurrentPage, query.PageSize, appErr = normalizePage(query.CurrentPage, query.PageSize)
	if appErr != nil {
		return nil, appErr
	}
	ok, err := repo.IsActiveParticipant(ctx, query.ConversationID, identity.UserID)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询会话权限失败", err)
	}
	if !ok {
		return nil, apperror.Forbidden("无权操作")
	}
	rows, total, err := repo.ListMessages(ctx, query)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询消息失败", err)
	}
	senderIDs := uniqueSenderIDs(rows)
	senders, err := repo.UserBriefs(ctx, senderIDs)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询发送者失败", err)
	}
	list := make([]MessageItem, 0, len(rows))
	for _, row := range rows {
		sender := senders[row.SenderID]
		list = append(list, messageItemFromMessage(row, &sender))
	}
	return &MessageListResponse{List: list, Page: Page{PageSize: query.PageSize, CurrentPage: query.CurrentPage, TotalPage: totalPage(total, query.PageSize), Total: total}}, nil
}

func (s *Service) MarkRead(ctx context.Context, identity Identity, input MarkReadInput) *apperror.Error {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	identity, appErr = normalizeIdentity(identity)
	if appErr != nil {
		return appErr
	}
	if input.ConversationID <= 0 {
		return apperror.BadRequest("会话ID无效")
	}
	ok, err := repo.IsActiveParticipant(ctx, input.ConversationID, identity.UserID)
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "查询会话权限失败", err)
	}
	if !ok {
		return apperror.Forbidden("无权操作")
	}
	messageID, err := repo.ConversationLastMessageID(ctx, input.ConversationID)
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "查询会话最后消息失败", err)
	}
	if messageID > 0 {
		if err := repo.UpdateLastReadMessageID(ctx, input.ConversationID, identity.UserID, messageID); err != nil {
			return apperror.Wrap(apperror.CodeInternal, 500, "标记已读失败", err)
		}
	}
	participants, err := repo.ActiveParticipantUserIDs(ctx, input.ConversationID)
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "查询会话成员失败", err)
	}
	s.publishToUsers(ctx, identity.Platform, participants, EventChatReadV1, fmt.Sprintf("chat-read-%d-%d", input.ConversationID, identity.UserID), map[string]any{"conversation_id": input.ConversationID, "user_id": identity.UserID})
	return nil
}

func (s *Service) DeleteConversation(ctx context.Context, identity Identity, conversationID int64) *apperror.Error {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	identity, appErr = normalizeIdentity(identity)
	if appErr != nil {
		return appErr
	}
	if conversationID <= 0 {
		return apperror.BadRequest("会话ID无效")
	}
	affected, err := repo.SoftDeleteConversationForUser(ctx, conversationID, identity.UserID)
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "删除会话失败", err)
	}
	if affected == 0 {
		return apperror.NotFound("会话不存在或已删除")
	}
	return nil
}

func (s *Service) TogglePin(ctx context.Context, identity Identity, conversationID int64) *apperror.Error {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	identity, appErr = normalizeIdentity(identity)
	if appErr != nil {
		return appErr
	}
	if conversationID <= 0 {
		return apperror.BadRequest("会话ID无效")
	}
	if err := repo.TogglePin(ctx, conversationID, identity.UserID); err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "切换会话置顶失败", err)
	}
	return nil
}

func (s *Service) requireRepository() (Repository, *apperror.Error) {
	if s == nil || s.repository == nil {
		return nil, apperror.Internal("聊天仓储未配置")
	}
	return s.repository, nil
}

func (s *Service) publishToUsers(ctx context.Context, platform string, userIDs []int64, eventType string, requestID string, payload map[string]any) {
	if s == nil || s.realtimePublisher == nil {
		return
	}
	platform = strings.TrimSpace(platform)
	if platform == "" {
		platform = enum.PlatformAdmin
	}
	ids := normalizePositiveIDs(userIDs)
	for _, userID := range ids {
		envelope, err := platformrealtime.NewEnvelope(eventType, requestID, payload)
		if err != nil {
			continue
		}
		if err := s.realtimePublisher.Publish(ctx, platformrealtime.Publication{Platform: platform, UserID: userID, Envelope: envelope}); err != nil && s.logger != nil && !errors.Is(err, platformrealtime.ErrSessionNotFound) {
			s.logger.WarnContext(ctx, "failed to publish chat realtime event", "type", eventType, "user_id", userID, "error", err)
		}
	}
}

func normalizeIdentity(identity Identity) (Identity, *apperror.Error) {
	if identity.UserID <= 0 {
		return identity, apperror.Unauthorized("Token无效或已过期")
	}
	identity.Platform = strings.TrimSpace(identity.Platform)
	if identity.Platform == "" {
		identity.Platform = enum.PlatformAdmin
	}
	return identity, nil
}

func normalizePage(currentPage, pageSize int) (int, int, *apperror.Error) {
	if currentPage <= 0 {
		return 0, 0, apperror.BadRequest("当前页无效")
	}
	if pageSize <= 0 {
		pageSize = defaultPageSize
	}
	if pageSize < enum.PageSizeMin || pageSize > enum.PageSizeMax {
		return 0, 0, apperror.BadRequest("每页数量无效")
	}
	return currentPage, pageSize, nil
}

func normalizeSendMessageInput(input SendMessageInput) (SendMessageInput, *apperror.Error) {
	if input.ConversationID <= 0 {
		return input, apperror.BadRequest("会话ID无效")
	}
	if !isMessageType(input.Type) {
		return input, apperror.BadRequest("消息类型无效")
	}
	input.Content = strings.TrimSpace(input.Content)
	if input.Type == MessageTypeText && input.Content == "" {
		return input, apperror.BadRequest("消息内容不能为空")
	}
	if input.Type != MessageTypeText && input.Content == "" {
		return input, apperror.BadRequest("消息内容不能为空")
	}
	if utf8.RuneCountInString(input.Content) > 5000 {
		return input, apperror.BadRequest("消息内容过长")
	}
	return input, nil
}

func isMessageType(value int) bool {
	return value == MessageTypeText || value == MessageTypeImage || value == MessageTypeFile || value == MessageTypeSystem
}

func conversationItemFromRow(row ConversationRow, unread int64) ConversationItem {
	announcement := ""
	if row.Announcement != nil {
		announcement = *row.Announcement
	}
	return ConversationItem{ID: row.ID, Type: row.Type, Name: row.Name, Avatar: row.Avatar, Announcement: announcement, OwnerID: row.OwnerID, LastMessageID: row.LastMessageID, LastMessageAt: formatTime(row.LastMessageAt), LastMessagePreview: row.LastMessagePreview, MemberCount: row.MemberCount, UnreadCount: unread, IsPinned: row.IsPinned, CreatedAt: formatTime(row.CreatedAt)}
}

func messageItemFromMessage(row Message, sender *UserBrief) MessageItem {
	item := MessageItem{ID: row.ID, ConversationID: row.ConversationID, SenderID: row.SenderID, Type: row.Type, Content: row.Content, CreatedAt: formatTime(row.CreatedAt)}
	if row.MetaJSON != nil && strings.TrimSpace(*row.MetaJSON) != "" {
		var meta map[string]any
		if err := json.Unmarshal([]byte(*row.MetaJSON), &meta); err == nil {
			item.MetaJSON = meta
		}
	}
	if sender != nil && sender.ID > 0 {
		item.Sender = sender
	}
	return item
}

func contactItemFromRow(row ContactRow) ContactItem {
	return ContactItem{ID: row.ID, ContactUserID: row.ContactUserID, Username: row.Username, Avatar: row.Avatar, Status: row.Status, IsInitiator: row.IsInitiator, IsOnline: false, CreatedAt: formatTime(row.CreatedAt)}
}

func messagePreview(messageType int, content string) string {
	switch messageType {
	case MessageTypeImage:
		return "[图片]"
	case MessageTypeFile:
		return "[文件]"
	default:
		return truncateRunes(content, 200)
	}
}

func truncateRunes(value string, max int) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= max {
		return string(runes)
	}
	return string(runes[:max])
}

func uniqueSenderIDs(rows []Message) []int64 {
	ids := make([]int64, 0, len(rows))
	seen := map[int64]struct{}{}
	for _, row := range rows {
		if row.SenderID > 0 {
			if _, ok := seen[row.SenderID]; !ok {
				seen[row.SenderID] = struct{}{}
				ids = append(ids, row.SenderID)
			}
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func normalizePositiveIDs(ids []int64) []int64 {
	seen := map[int64]struct{}{}
	result := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func totalPage(total int64, pageSize int) int {
	if total <= 0 || pageSize <= 0 {
		return 0
	}
	return int(math.Ceil(float64(total) / float64(pageSize)))
}

const timeLayout = "2006-01-02 15:04:05"

var (
	errContactExists          = errors.New("contact exists")
	errContactNotFound        = errors.New("contact not found")
	errContactAlreadyHandled  = errors.New("contact already handled")
	errConfirmOutgoingContact = errors.New("cannot confirm outgoing contact")
)

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format(timeLayout)
}
