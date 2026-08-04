package aiconversation

import (
	"context"
	"errors"
	"testing"
	"time"

	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/enum"
)

type fakeRepository struct {
	rows                     []ListRow
	hasMore                  bool
	row                      *Conversation
	activeAgents             map[int64]bool
	visibleAssistantMessages map[int64]bool
	unreadCounts             map[int64]uint64
	listQuery                ListQuery
	unreadCountQueries       [][]int64
	created                  Conversation
	deleteID                 int64
	deleteUserID             int64
	deleteResult             DeleteResult
	updateID                 int64
	updateUserID             int64
	updateTitle              string
	cursorMessageIDs         []int64
	cursorErr                error
}

func (f *fakeRepository) List(ctx context.Context, query ListQuery) ([]ListRow, bool, error) {
	f.listQuery = query
	return f.rows, f.hasMore, nil
}
func (f *fakeRepository) UnreadCounts(ctx context.Context, conversationIDs []int64) (map[int64]uint64, error) {
	f.unreadCountQueries = append(f.unreadCountQueries, append([]int64(nil), conversationIDs...))
	return f.unreadCounts, nil
}
func (f *fakeRepository) Get(ctx context.Context, id int64) (*Conversation, string, error) {
	if f.row == nil {
		return nil, "", nil
	}
	return f.row, "客服助手", nil
}
func (f *fakeRepository) ActiveChatAgentExists(ctx context.Context, id int64) (bool, error) {
	return f.activeAgents[id], nil
}
func (f *fakeRepository) Create(ctx context.Context, row Conversation) (int64, error) {
	f.created = row
	return 9, nil
}
func (f *fakeRepository) UpdateTitle(ctx context.Context, id int64, userID int64, title string) error {
	f.updateID = id
	f.updateUserID = userID
	f.updateTitle = title
	return nil
}

func (f *fakeRepository) Delete(ctx context.Context, id int64, userID int64) (DeleteResult, error) {
	f.deleteID = id
	f.deleteUserID = userID
	return f.deleteResult, nil
}

type fakeCancelPublisher struct {
	commandIDs []uint64
}

func (f *fakeCancelPublisher) PublishCancel(_ context.Context, commandID uint64) error {
	f.commandIDs = append(f.commandIDs, commandID)
	return nil
}

func (f *fakeRepository) AdvanceReadCursor(ctx context.Context, conversationID int64, userID int64, messageID int64) (int64, uint64, bool, error) {
	f.cursorMessageIDs = append(f.cursorMessageIDs, messageID)
	if f.cursorErr != nil {
		return 0, 0, false, f.cursorErr
	}
	if f.row == nil || f.row.ID != conversationID || f.row.UserID != userID || !f.visibleAssistantMessages[messageID] {
		return 0, 0, false, nil
	}
	if messageID > f.row.LastReadMessageID {
		f.row.LastReadMessageID = messageID
	}
	return f.row.LastReadMessageID, f.unreadCounts[conversationID], true, nil
}

func TestListUsesCursorLimitAndDoesNotExposeUserOrStatus(t *testing.T) {
	now := time.Date(2026, 5, 9, 10, 0, 0, 0, time.UTC)
	repo := &fakeRepository{
		rows:         []ListRow{{Conversation: Conversation{ID: 1, UserID: 7, AgentID: 3, Title: "hello", LastMessageAt: &now, IsDel: enum.CommonNo, UpdatedAt: now}, AgentName: "客服助手"}},
		unreadCounts: map[int64]uint64{1: 2},
	}
	res, appErr := NewService(repo).List(context.Background(), 7, ListQuery{AgentID: ptrInt64(3), BeforeTime: &now, BeforeID: 20, Limit: 0})
	if appErr != nil {
		t.Fatalf("List returned error: %v", appErr)
	}
	if repo.listQuery.UserID != 7 || repo.listQuery.BeforeTime == nil || repo.listQuery.BeforeID != 20 || repo.listQuery.Limit != 20 || repo.listQuery.AgentID == nil || *repo.listQuery.AgentID != 3 {
		t.Fatalf("unexpected normalized query: %#v", repo.listQuery)
	}
	if len(res.List) != 1 || res.List[0].AgentName != "客服助手" || res.List[0].UnreadCount != 2 || res.List[0].LastMessageAt == "" || res.NextID != 0 || res.HasMore {
		t.Fatalf("unexpected list response: %#v", res)
	}
	if len(repo.unreadCountQueries) != 1 || len(repo.unreadCountQueries[0]) != 1 || repo.unreadCountQueries[0][0] != 1 {
		t.Fatalf("unread counts must use one page query, got %#v", repo.unreadCountQueries)
	}
}

func TestListUnreadCountsTwoConversationsInOneQuery(t *testing.T) {
	now := time.Date(2026, 5, 9, 10, 0, 0, 0, time.UTC)
	repo := &fakeRepository{
		rows: []ListRow{
			{Conversation: Conversation{ID: 8, UserID: 7, LastMessageAt: &now}},
			{Conversation: Conversation{ID: 6, UserID: 7, LastMessageAt: &now}},
		},
		unreadCounts: map[int64]uint64{8: 3},
	}
	res, appErr := NewService(repo).List(context.Background(), 7, ListQuery{})
	if appErr != nil {
		t.Fatalf("List returned error: %v", appErr)
	}
	if len(repo.unreadCountQueries) != 1 || len(repo.unreadCountQueries[0]) != 2 || repo.unreadCountQueries[0][0] != 8 || repo.unreadCountQueries[0][1] != 6 {
		t.Fatalf("unread counts must be grouped for the current page: %#v", repo.unreadCountQueries)
	}
	if len(res.List) != 2 || res.List[0].UnreadCount != 3 || res.List[1].UnreadCount != 0 {
		t.Fatalf("unexpected unread projections: %#v", res.List)
	}
}

func TestListUnreadSkipsCountQueryForEmptyPage(t *testing.T) {
	repo := &fakeRepository{}
	res, appErr := NewService(repo).List(context.Background(), 7, ListQuery{})
	if appErr != nil {
		t.Fatalf("List returned error: %v", appErr)
	}
	if len(repo.unreadCountQueries) != 0 || len(res.List) != 0 {
		t.Fatalf("empty page must not query unread counts: calls=%#v response=%#v", repo.unreadCountQueries, res)
	}
}

func TestListUsesStableLastMessageTimeAndIDCursor(t *testing.T) {
	cursorTime := time.Date(2026, 5, 9, 10, 0, 0, 0, time.UTC)
	nextTime := cursorTime.Add(-time.Minute)
	repo := &fakeRepository{
		hasMore: true,
		rows: []ListRow{{
			Conversation: Conversation{ID: 19, UserID: 7, AgentID: 3, LastMessageAt: &nextTime, UpdatedAt: nextTime},
		}},
	}

	res, appErr := NewService(repo).List(context.Background(), 7, ListQuery{BeforeTime: &cursorTime, BeforeID: 20, Limit: 20})
	if appErr != nil {
		t.Fatalf("List returned error: %v", appErr)
	}
	if repo.listQuery.BeforeTime == nil || !repo.listQuery.BeforeTime.Equal(cursorTime) || repo.listQuery.BeforeID != 20 {
		t.Fatalf("repository did not receive tuple cursor: %#v", repo.listQuery)
	}
	if res.NextTime != "2026-05-09 09:59:00" || res.NextID != 19 || !res.HasMore {
		t.Fatalf("unexpected next tuple cursor: %#v", res)
	}
}

func TestListRejectsPartialCursorWithStableMessageID(t *testing.T) {
	_, appErr := NewService(&fakeRepository{}).List(context.Background(), 7, ListQuery{BeforeID: 20})
	if appErr == nil || appErr.MessageID != "aiconversation.cursor.invalid" {
		t.Fatalf("expected stable cursor message ID, got %#v", appErr)
	}
}

func TestDetailRejectsConversationNotOwnedByCurrentUser(t *testing.T) {
	repo := &fakeRepository{row: &Conversation{ID: 3, UserID: 8, AgentID: 1, Title: "other", IsDel: enum.CommonNo}}
	_, appErr := NewService(repo).Detail(context.Background(), 7, 3)
	if appErr == nil || appErr.LegacyCode != 403 {
		t.Fatalf("expected forbidden, got %#v", appErr)
	}
}

func TestCreateValidatesChatAgentAndSetsCurrentUser(t *testing.T) {
	repo := &fakeRepository{activeAgents: map[int64]bool{5: true}}
	id, appErr := NewService(repo).Create(context.Background(), 7, CreateInput{AgentID: 5, Title: "  New chat  "})
	if appErr != nil {
		t.Fatalf("Create returned error: %v", appErr)
	}
	if id != 9 || repo.created.UserID != 7 || repo.created.AgentID != 5 || repo.created.Title != "New chat" || repo.created.IsDel != enum.CommonNo || repo.created.LastMessageAt == nil {
		t.Fatalf("unexpected created row: id=%d row=%#v", id, repo.created)
	}
}

func TestCreateRejectsNonChatAgent(t *testing.T) {
	repo := &fakeRepository{activeAgents: map[int64]bool{5: false}}
	_, appErr := NewService(repo).Create(context.Background(), 7, CreateInput{AgentID: 5})
	if appErr == nil || appErr.LegacyCode != 100 || appErr.Message != "该智能体不支持对话场景" {
		t.Fatalf("expected non-chat bad request, got %#v", appErr)
	}
}

func TestUpdateRequiresOwnerAndTrimsTitle(t *testing.T) {
	repo := &fakeRepository{row: &Conversation{ID: 3, UserID: 7, IsDel: enum.CommonNo}}
	if appErr := NewService(repo).Update(context.Background(), 7, 3, UpdateInput{Title: "  new title  "}); appErr != nil {
		t.Fatalf("Update returned error: %v", appErr)
	}
	if repo.updateID != 3 || repo.updateUserID != 7 || repo.updateTitle != "new title" {
		t.Fatalf("unexpected update call: id=%d user=%d title=%q", repo.updateID, repo.updateUserID, repo.updateTitle)
	}
}

func TestUpdateRejectsBlankTitle(t *testing.T) {
	repo := &fakeRepository{row: &Conversation{ID: 3, UserID: 7, IsDel: enum.CommonNo}}
	appErr := NewService(repo).Update(context.Background(), 7, 3, UpdateInput{Title: "  "})
	if appErr == nil || appErr.Message != "AI会话标题不能为空" {
		t.Fatalf("expected blank title rejection, got %#v", appErr)
	}
}

func TestDeleteRequiresOwnerAndSoftDeletesMessages(t *testing.T) {
	repo := &fakeRepository{
		row:          &Conversation{ID: 3, UserID: 7, IsDel: enum.CommonNo},
		deleteResult: DeleteResult{CanceledCommandIDs: []uint64{41, 42}},
	}
	publisher := &fakeCancelPublisher{}
	if appErr := NewService(repo, WithCancelPublisher(publisher)).Delete(context.Background(), 7, 3); appErr != nil {
		t.Fatalf("Delete returned error: %v", appErr)
	}
	if repo.deleteID != 3 || repo.deleteUserID != 7 {
		t.Fatalf("unexpected delete call: id=%d user=%d", repo.deleteID, repo.deleteUserID)
	}
	if len(publisher.commandIDs) != 2 || publisher.commandIDs[0] != 41 || publisher.commandIDs[1] != 42 {
		t.Fatalf("cancel signals=%v", publisher.commandIDs)
	}
}

func TestReadCursorAdvancesMonotonicallyAndReturnsFreshUnreadCount(t *testing.T) {
	repo := &fakeRepository{
		row:                      &Conversation{ID: 3, UserID: 7, LastReadMessageID: 4, IsDel: enum.CommonNo},
		visibleAssistantMessages: map[int64]bool{9: true, 7: true},
		unreadCounts:             map[int64]uint64{3: 2},
	}
	service := NewService(repo)

	first, appErr := service.AdvanceReadCursor(context.Background(), 7, 3, 9)
	if appErr != nil {
		t.Fatalf("first cursor update returned error: %v", appErr)
	}
	repeated, appErr := service.AdvanceReadCursor(context.Background(), 7, 3, 9)
	if appErr != nil {
		t.Fatalf("repeated cursor update returned error: %v", appErr)
	}
	backward, appErr := service.AdvanceReadCursor(context.Background(), 7, 3, 7)
	if appErr != nil {
		t.Fatalf("backward cursor update returned error: %v", appErr)
	}
	for _, result := range []*ReadCursorResponse{first, repeated, backward} {
		if result.ConversationID != 3 || result.LastReadMessageID != 9 || result.UnreadCount != 2 {
			t.Fatalf("unexpected persisted cursor response: %#v", result)
		}
	}
	if len(repo.cursorMessageIDs) != 3 || repo.cursorMessageIDs[0] != 9 || repo.cursorMessageIDs[1] != 9 || repo.cursorMessageIDs[2] != 7 {
		t.Fatalf("unexpected cursor requests: %#v", repo.cursorMessageIDs)
	}
	if len(repo.unreadCountQueries) != 0 {
		t.Fatalf("cursor repository result must avoid a separate unread query: calls=%#v", repo.unreadCountQueries)
	}
}

func TestReadCursorPreservesRequestCancellation(t *testing.T) {
	repo := &fakeRepository{cursorErr: context.Canceled}
	_, appErr := NewService(repo).AdvanceReadCursor(context.Background(), 7, 3, 9)
	if appErr == nil {
		t.Fatal("expected canceled read cursor error")
	}
	if appErr.Category != apperror.CategoryCanceled || appErr.HTTPStatus != 408 || appErr.Code != "request.canceled" {
		t.Fatalf("request cancellation was reclassified: %#v", appErr)
	}
	if !errors.Is(appErr, context.Canceled) {
		t.Fatalf("canceled cause was not preserved: %v", appErr)
	}
}

func TestReadCursorRejectsForeignHiddenOrUserMessage(t *testing.T) {
	for _, test := range []struct {
		name      string
		messageID int64
	}{
		{name: "foreign conversation", messageID: 11},
		{name: "hidden assistant", messageID: 12},
		{name: "user role", messageID: 13},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo := &fakeRepository{
				row:                      &Conversation{ID: 3, UserID: 7, IsDel: enum.CommonNo},
				visibleAssistantMessages: map[int64]bool{},
			}
			_, appErr := NewService(repo).AdvanceReadCursor(context.Background(), 7, 3, test.messageID)
			if appErr == nil || appErr.HTTPStatus != 404 || appErr.MessageID != "aiconversation.read_cursor.message_invalid" {
				t.Fatalf("expected ownership-safe message rejection, got %#v", appErr)
			}
			if len(repo.unreadCountQueries) != 0 {
				t.Fatalf("invalid cursor target queried unread counts: %#v", repo.unreadCountQueries)
			}
		})
	}
}

func TestReadCursorRejectsConversationNotOwnedByCurrentUser(t *testing.T) {
	repo := &fakeRepository{
		row:                      &Conversation{ID: 3, UserID: 8, IsDel: enum.CommonNo},
		visibleAssistantMessages: map[int64]bool{9: true},
	}
	_, appErr := NewService(repo).AdvanceReadCursor(context.Background(), 7, 3, 9)
	if appErr == nil || appErr.HTTPStatus != 404 || appErr.MessageID != "aiconversation.read_cursor.message_invalid" {
		t.Fatalf("expected ownership-safe conversation rejection, got %#v", appErr)
	}
	if len(repo.unreadCountQueries) != 0 {
		t.Fatalf("ownership rejection queried unread counts: %#v", repo.unreadCountQueries)
	}
}

func ptrInt64(v int64) *int64 { return &v }
