package task

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strings"
	"time"

	"admin_back_go/internal/infra/taskqueue"
	modulerealtime "admin_back_go/internal/module/realtime"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/dict"
	"admin_back_go/internal/shared/enum"
)

const (
	defaultDispatchLimit = 100
	sendBatchSize        = 100
	defaultWorkLeaseTTL  = 30 * time.Second

	EventNotificationCreatedV1 = modulerealtime.TypeNotificationCreatedV1
)

var ErrLeaseLost = errors.New("notification task lease lost")

type Service struct {
	repository Repository
	enqueuer   taskqueue.Enqueuer
	logger     *slog.Logger
	now        func() time.Time
	owner      string
	leaseTTL   time.Duration
}

type Option func(*Service)

func WithEnqueuer(enqueuer taskqueue.Enqueuer) Option {
	return func(s *Service) {
		s.enqueuer = enqueuer
	}
}

func WithLogger(logger *slog.Logger) Option {
	return func(s *Service) {
		if logger != nil {
			s.logger = logger
		}
	}
}

func WithNow(now func() time.Time) Option {
	return func(s *Service) {
		if now != nil {
			s.now = now
		}
	}
}

func WithWorkLease(owner string, ttl time.Duration) Option {
	return func(s *Service) {
		if strings.TrimSpace(owner) != "" {
			s.owner = strings.TrimSpace(owner)
		}
		if ttl > 0 {
			s.leaseTTL = ttl
		}
	}
}

func NewService(repository Repository, options ...Option) *Service {
	service := &Service{
		repository: repository,
		logger:     slog.Default(),
		now:        time.Now,
		owner:      newWorkOwner("notification"),
		leaseTTL:   defaultWorkLeaseTTL,
	}
	for _, option := range options {
		if option != nil {
			option(service)
		}
	}
	return service
}

func (s *Service) PageInit(ctx context.Context) (*InitResponse, *apperror.Error) {
	return &InitResponse{Dict: InitDict{
		NotificationTypeArr:       dict.NotificationTypeOptions(),
		NotificationLevelArr:      dict.NotificationLevelOptions(),
		NotificationTargetTypeArr: dict.NotificationTargetTypeOptions(),
		NotificationTaskStatusArr: dict.NotificationTaskStatusOptions(),
		PlatformArr:               dict.NotificationTaskPlatformOptions(),
	}}, nil
}

func (s *Service) StatusCount(ctx context.Context, query StatusCountQuery) ([]StatusCountItem, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	query.Title = strings.TrimSpace(query.Title)
	counts, err := repo.CountByStatus(ctx, query)
	if err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "notificationtask.status_count_failed", nil, "查询通知任务状态统计失败", err)
	}
	items := dict.NotificationTaskStatusOptions()
	result := make([]StatusCountItem, 0, len(items))
	for _, item := range items {
		result = append(result, StatusCountItem{Label: item.Label, Value: item.Value, Num: counts[item.Value]})
	}
	return result, nil
}

func (s *Service) List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	query, appErr = normalizeListQuery(query)
	if appErr != nil {
		return nil, appErr
	}
	rows, total, err := repo.List(ctx, query)
	if err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "notificationtask.query_failed", nil, "查询通知任务失败", err)
	}
	list := make([]ListItem, 0, len(rows))
	for _, row := range rows {
		list = append(list, listItemFromTask(row))
	}
	return &ListResponse{
		List: list,
		Page: Page{PageSize: query.PageSize, CurrentPage: query.CurrentPage, TotalPage: totalPage(total, query.PageSize), Total: total},
	}, nil
}

func (s *Service) Create(ctx context.Context, input CreateInput) (*CreateResponse, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	input, sendAt, appErr := normalizeCreateInput(input, s.now())
	if appErr != nil {
		return nil, appErr
	}
	totalCount, err := repo.CountTargetUsers(ctx, input.TargetType, input.TargetIDs)
	if err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "notificationtask.target_count_failed", nil, "计算通知目标用户失败", err)
	}
	targetIDsJSON, err := json.Marshal(input.TargetIDs)
	if err != nil {
		return nil, apperror.BadRequestKey("notificationtask.target.invalid", nil, "通知目标格式错误")
	}
	id, err := repo.Create(ctx, Task{
		Title:      input.Title,
		Content:    input.Content,
		Type:       input.Type,
		Level:      input.Level,
		Link:       input.Link,
		Platform:   input.Platform,
		TargetType: input.TargetType,
		TargetIDs:  string(targetIDsJSON),
		Status:     enum.NotificationTaskStatusPending,
		TotalCount: totalCount,
		SendAt:     sendAt,
		CreatedBy:  input.CreatedBy,
		IsDel:      enum.CommonNo,
	})
	if err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "notificationtask.create_failed", nil, "创建通知任务失败", err)
	}
	if sendAt != nil {
		return &CreateResponse{ID: id, Queued: false}, nil
	}
	task, err := NewSendTask(id)
	if err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "notificationtask.send_task_build_failed", nil, "构建通知发送任务失败", err)
	}
	if _, err := s.enqueue(ctx, task); err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "notificationtask.enqueue_failed", nil, "通知任务已创建但入队失败，请等待调度器补偿或手动处理", err)
	}
	return &CreateResponse{ID: id, Queued: true}, nil
}

func (s *Service) Cancel(ctx context.Context, id int64) *apperror.Error {
	if id <= 0 {
		return apperror.BadRequestKey("notificationtask.id.invalid", nil, "无效的通知任务ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	row, err := repo.Get(ctx, id)
	if err != nil {
		return apperror.WrapKey(apperror.CodeInternal, 500, "notificationtask.query_failed", nil, "查询通知任务失败", err)
	}
	if row == nil {
		return apperror.NotFoundKey("notificationtask.not_found", nil, "通知任务不存在")
	}
	if row.Status != enum.NotificationTaskStatusPending {
		return apperror.BadRequestKey("notificationtask.cancel.pending_only", nil, "只能取消待发送的通知任务")
	}
	affected, err := repo.CancelPending(ctx, id)
	if err != nil {
		return apperror.WrapKey(apperror.CodeInternal, 500, "notificationtask.cancel_failed", nil, "取消通知任务失败", err)
	}
	if affected == 0 {
		return apperror.BadRequestKey("notificationtask.state_changed", nil, "任务状态已变更，请刷新后重试")
	}
	return nil
}

func (s *Service) Delete(ctx context.Context, id int64) *apperror.Error {
	if id <= 0 {
		return apperror.BadRequestKey("notificationtask.id.invalid", nil, "无效的通知任务ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	row, err := repo.Get(ctx, id)
	if err != nil {
		return apperror.WrapKey(apperror.CodeInternal, 500, "notificationtask.query_failed", nil, "查询通知任务失败", err)
	}
	if row == nil {
		return apperror.NotFoundKey("notificationtask.not_found", nil, "通知任务不存在")
	}
	affected, err := repo.Delete(ctx, id)
	if err != nil {
		return apperror.WrapKey(apperror.CodeInternal, 500, "notificationtask.delete_failed", nil, "删除通知任务失败", err)
	}
	if affected == 0 {
		return apperror.BadRequestKey("notificationtask.state_changed", nil, "通知任务状态已变更，请刷新后重试")
	}
	return nil
}

func (s *Service) DispatchDue(ctx context.Context, input DispatchDueInput) (*DispatchDueResult, error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	if input.Now.IsZero() {
		input.Now = s.now()
	}
	if input.Limit <= 0 {
		input.Limit = defaultDispatchLimit
	}
	ids, err := repo.ListDueTaskIDs(ctx, input.Now, input.Limit)
	if err != nil {
		return nil, fmt.Errorf("list due notification tasks: %w", err)
	}
	result := &DispatchDueResult{Claimed: len(ids)}
	for _, id := range ids {
		task, err := NewSendTask(id)
		if err != nil {
			return nil, err
		}
		if _, err := s.enqueue(ctx, task); err != nil {
			return nil, fmt.Errorf("enqueue notification task %d: %w", id, err)
		}
		result.Queued++
	}
	return result, nil
}

func (s *Service) SendTask(ctx context.Context, input SendTaskInput) (*SendTaskResult, error) {
	if input.TaskID <= 0 {
		return nil, apperror.BadRequestKey("notificationtask.id.invalid", nil, "无效的通知任务ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	claim, err := repo.ClaimByID(ctx, input.TaskID, s.owner, s.now(), s.leaseTTL)
	if err != nil {
		return nil, fmt.Errorf("claim notification task %d: %w", input.TaskID, err)
	}
	if claim == nil {
		return &SendTaskResult{TaskID: input.TaskID, Noop: true}, nil
	}

	runCtx, stopRenewal := s.startLeaseRenewal(ctx, repo, claim)
	result, workErr := s.deliverClaim(runCtx, repo, claim)
	leaseLost, renewErr := stopRenewal()
	if leaseLost {
		return nil, errors.Join(ErrLeaseLost, renewErr)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if errors.Is(workErr, ErrLeaseLost) {
		return nil, workErr
	}
	if workErr != nil {
		now := s.now()
		marked, markErr := repo.MarkFailed(context.WithoutCancel(ctx), claim.Task.ID, claim.Owner, claim.Token, now, workErr.Error())
		if markErr != nil {
			return nil, errors.Join(workErr, markErr)
		}
		if !marked {
			return nil, errors.Join(workErr, ErrLeaseLost)
		}
		return nil, workErr
	}
	now := s.now()
	marked, err := repo.MarkSuccess(ctx, claim.Task.ID, claim.Owner, claim.Token, now, result.Sent, result.Sent)
	if err != nil {
		return nil, fmt.Errorf("mark notification task %d success: %w", input.TaskID, err)
	}
	if !marked {
		return nil, ErrLeaseLost
	}
	return result, nil
}

func (s *Service) deliverClaim(ctx context.Context, repo Repository, claim *Claim) (*SendTaskResult, error) {
	task := claim.Task
	if !enum.IsNotificationAudiencePlatform(strings.TrimSpace(task.Platform)) {
		return nil, fmt.Errorf("notification task %d platform is not registered: %q", task.ID, task.Platform)
	}

	userIDs, err := repo.TargetUserIDs(ctx, task)
	if err != nil {
		return nil, fmt.Errorf("resolve notification task %d target users: %w", task.ID, err)
	}
	totalCount := len(userIDs)
	sentCount := 0
	for start := 0; start < len(userIDs); start += sendBatchSize {
		end := start + sendBatchSize
		if end > len(userIDs) {
			end = len(userIDs)
		}
		rows := make([]Notification, 0, end-start)
		for _, userID := range userIDs[start:end] {
			rows = append(rows, Notification{
				SourceTaskID: task.ID,
				UserID:       userID,
				Title:        task.Title,
				Content:      task.Content,
				Type:         task.Type,
				Level:        task.Level,
				Link:         task.Link,
				Platform:     task.Platform,
				IsRead:       enum.CommonNo,
				IsDel:        enum.CommonNo,
			})
		}
		if err := repo.InsertNotifications(ctx, rows); err != nil {
			return nil, fmt.Errorf("insert notification task %d batch: %w", task.ID, err)
		}
		sentCount += len(rows)
		now := s.now()
		updated, err := repo.UpdateProgress(ctx, task.ID, claim.Owner, claim.Token, now, sentCount, totalCount)
		if err != nil {
			return nil, fmt.Errorf("update notification task %d progress: %w", task.ID, err)
		}
		if !updated {
			return nil, ErrLeaseLost
		}
	}
	return &SendTaskResult{TaskID: task.ID, Sent: sentCount}, nil
}

func (s *Service) startLeaseRenewal(ctx context.Context, repo Repository, claim *Claim) (context.Context, func() (bool, error)) {
	if ctx == nil {
		ctx = context.Background()
	}
	runCtx, cancelRun := context.WithCancelCause(ctx)
	renewCtx, cancelRenew := context.WithCancel(ctx)
	stop := make(chan struct{})
	done := make(chan struct{})
	lost := false
	var renewErr error
	go func() {
		defer close(done)
		interval := s.leaseTTL / 3
		if interval < time.Millisecond {
			interval = time.Millisecond
		}
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-renewCtx.Done():
				return
			case <-ticker.C:
				now := s.now()
				alive, err := repo.Renew(renewCtx, claim.Task.ID, claim.Owner, claim.Token, now, now.Add(s.leaseTTL))
				if err != nil || !alive {
					select {
					case <-stop:
						return
					default:
					}
					lost = true
					renewErr = err
					cancelRun(ErrLeaseLost)
					return
				}
			}
		}
	}()
	return runCtx, func() (bool, error) {
		close(stop)
		cancelRenew()
		<-done
		cancelRun(nil)
		return lost, renewErr
	}
}

func newWorkOwner(prefix string) string {
	var random [8]byte
	if _, err := rand.Read(random[:]); err == nil {
		return prefix + "-" + hex.EncodeToString(random[:])
	}
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}

func (s *Service) requireRepository() (Repository, *apperror.Error) {
	if s == nil || s.repository == nil {
		return nil, apperror.InternalKey("notificationtask.repository_missing", nil, "通知任务仓储未配置")
	}
	return s.repository, nil
}

func (s *Service) enqueue(ctx context.Context, task taskqueue.Task) (taskqueue.EnqueueResult, error) {
	if s == nil || s.enqueuer == nil {
		return taskqueue.EnqueueResult{}, taskqueue.ErrClientNotReady
	}
	return s.enqueuer.Enqueue(ctx, task)
}

func normalizeListQuery(query ListQuery) (ListQuery, *apperror.Error) {
	if query.CurrentPage <= 0 {
		return query, apperror.BadRequestKey("notificationtask.current_page.invalid", nil, "当前页无效")
	}
	if query.PageSize < enum.PageSizeMin || query.PageSize > enum.PageSizeMax {
		return query, apperror.BadRequestKey("notificationtask.page_size.invalid", nil, "每页数量无效")
	}
	if query.Status != nil && !enum.IsNotificationTaskStatus(*query.Status) {
		return query, apperror.BadRequestKey("notificationtask.status.invalid", nil, "无效的通知任务状态")
	}
	query.Title = strings.TrimSpace(query.Title)
	return query, nil
}

func normalizeCreateInput(input CreateInput, now time.Time) (CreateInput, *time.Time, *apperror.Error) {
	input.Title = strings.TrimSpace(input.Title)
	if input.Title == "" || len([]rune(input.Title)) > 100 {
		return input, nil, apperror.BadRequestKey("notificationtask.title.invalid", nil, "通知标题不能为空且不能超过100个字符")
	}
	input.Content = strings.TrimSpace(input.Content)
	input.Link = strings.TrimSpace(input.Link)
	if len([]rune(input.Link)) > 500 {
		return input, nil, apperror.BadRequestKey("notificationtask.link.too_long", nil, "通知链接不能超过500个字符")
	}
	if input.Type == 0 {
		input.Type = enum.NotificationTypeInfo
	}
	if !enum.IsNotificationType(input.Type) {
		return input, nil, apperror.BadRequestKey("notificationtask.type.invalid", nil, "无效的通知类型")
	}
	if input.Level == 0 {
		input.Level = enum.NotificationLevelNormal
	}
	if !enum.IsNotificationLevel(input.Level) {
		return input, nil, apperror.BadRequestKey("notificationtask.level.invalid", nil, "无效的通知级别")
	}
	input.Platform = strings.TrimSpace(input.Platform)
	if input.Platform == "" {
		input.Platform = enum.PlatformAll
	}
	if !enum.IsNotificationAudiencePlatform(input.Platform) {
		return input, nil, apperror.BadRequestKey("notificationtask.platform.invalid", nil, "无效的平台标识")
	}
	if !enum.IsNotificationTargetType(input.TargetType) {
		return input, nil, apperror.BadRequestKey("notificationtask.target_type.invalid", nil, "无效的通知目标类型")
	}
	input.TargetIDs = normalizeIDs(input.TargetIDs)
	switch input.TargetType {
	case enum.NotificationTargetAll:
		input.TargetIDs = []int64{}
	case enum.NotificationTargetUsers, enum.NotificationTargetRoles:
		if len(input.TargetIDs) == 0 {
			return input, nil, apperror.BadRequestKey("notificationtask.target.required", nil, "请选择通知目标")
		}
	}
	if input.CreatedBy <= 0 {
		return input, nil, apperror.UnauthorizedKey("auth.token.invalid_or_expired", nil, "Token无效或已过期")
	}

	sendAtText := strings.TrimSpace(input.SendAt)
	if sendAtText == "" {
		return input, nil, nil
	}
	sendAt, err := time.ParseInLocation(timeLayout, sendAtText, time.Local)
	if err != nil {
		return input, nil, apperror.BadRequestKey("notificationtask.send_at.format_invalid", nil, "定时发送时间格式错误")
	}
	if !now.IsZero() && sendAt.Before(now.Add(-time.Second)) {
		return input, nil, apperror.BadRequestKey("notificationtask.send_at.past", nil, "定时发送时间不能早于当前时间")
	}
	return input, &sendAt, nil
}

func normalizeIDs(ids []int64) []int64 {
	if len(ids) == 0 {
		return []int64{}
	}
	seen := make(map[int64]struct{}, len(ids))
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

func decodeIDs(raw string) []int64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []int64{}
	}
	var ids []int64
	if err := json.Unmarshal([]byte(raw), &ids); err != nil {
		return []int64{}
	}
	return normalizeIDs(ids)
}

func listItemFromTask(row Task) ListItem {
	return ListItem{
		ID: row.ID, Title: row.Title, Content: row.Content, Type: row.Type, TypeText: enum.NotificationTypeLabels[row.Type],
		Level: row.Level, LevelText: enum.NotificationLevelLabels[row.Level], Link: row.Link,
		Platform: row.Platform, PlatformText: platformText(row.Platform),
		TargetType: row.TargetType, TargetTypeText: enum.NotificationTargetTypeLabels[row.TargetType],
		Status: row.Status, StatusText: enum.NotificationTaskStatusLabels[row.Status],
		TotalCount: row.TotalCount, SentCount: row.SentCount, SendAt: optionalTime(row.SendAt),
		ErrorMsg: optionalString(row.ErrorMsg), CreatedAt: formatTime(row.CreatedAt),
	}
}

func optionalTime(value *time.Time) *string {
	if value == nil || value.IsZero() {
		return nil
	}
	text := formatTime(*value)
	return &text
}

func optionalString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func platformText(platform string) string {
	switch platform {
	case enum.PlatformAll:
		return "全平台"
	case enum.PlatformAdmin:
		return enum.PlatformAdmin
	default:
		return platform
	}
}

func totalPage(total int64, pageSize int) int {
	if total <= 0 || pageSize <= 0 {
		return 0
	}
	return int(math.Ceil(float64(total) / float64(pageSize)))
}

func truncateRunes(value string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= max {
		return string(runes)
	}
	return string(runes[:max])
}

func realtimePlatform(platform string) string {
	switch strings.TrimSpace(platform) {
	case enum.PlatformAdmin, enum.PlatformAll:
		return enum.PlatformAdmin
	default:
		return ""
	}
}

func notificationLevelKey(level int) (string, bool) {
	switch level {
	case enum.NotificationLevelNormal:
		return "normal", true
	case enum.NotificationLevelUrgent:
		return "urgent", true
	default:
		return "", false
	}
}

func notificationTypeKey(value int) (string, bool) {
	switch value {
	case enum.NotificationTypeInfo:
		return "info", true
	case enum.NotificationTypeSuccess:
		return "success", true
	case enum.NotificationTypeWarning:
		return "warning", true
	case enum.NotificationTypeError:
		return "error", true
	default:
		return "", false
	}
}
