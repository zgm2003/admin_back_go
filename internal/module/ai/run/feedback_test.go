package airun

import (
	"context"
	"testing"
	"time"

	"admin_back_go/internal/shared/clock"
	"admin_back_go/internal/shared/enum"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type fakeFeedbackRepository struct {
	run     *FeedbackRun
	setRuns []int64
	setLike []bool
}

func (f *fakeFeedbackRepository) FeedbackRun(ctx context.Context, id int64) (*FeedbackRun, error) {
	if f.run == nil || f.run.ID != id {
		return nil, nil
	}
	copy := *f.run
	return &copy, nil
}

func (f *fakeFeedbackRepository) UpdateUserFeedback(ctx context.Context, id int64, userID int64, liked bool, now time.Time) (*time.Time, bool, error) {
	f.setRuns = append(f.setRuns, id)
	f.setLike = append(f.setLike, liked)
	if f.run == nil || f.run.ID != id || f.run.UserID != userID {
		return nil, false, nil
	}
	if liked {
		if f.run.LikedAt == nil {
			value := now
			f.run.LikedAt = &value
		}
	} else {
		f.run.LikedAt = nil
	}
	return cloneTime(f.run.LikedAt), true, nil
}

func TestFeedbackLikedTrueAndFalseAreIdempotent(t *testing.T) {
	firstNow := time.Date(2026, 7, 27, 12, 1, 2, 345000000, time.UTC)
	clockReads := 0
	repository := &fakeFeedbackRepository{run: eligibleFeedbackRun(44, 7)}
	service := NewService(&fakeRepository{}, WithFeedbackRepository(repository), WithClock(clock.Func(func() time.Time {
		clockReads++
		return firstNow.Add(time.Duration(clockReads-1) * time.Minute)
	})))

	first, appErr := service.SetUserFeedback(context.Background(), 7, 44, true)
	if appErr != nil {
		t.Fatalf("first like returned error: %v", appErr)
	}
	repeated, appErr := service.SetUserFeedback(context.Background(), 7, 44, true)
	if appErr != nil {
		t.Fatalf("repeated like returned error: %v", appErr)
	}
	if !first.Liked || first.LikedAt == nil || !repeated.Liked || repeated.LikedAt == nil || *first.LikedAt != *repeated.LikedAt {
		t.Fatalf("repeated like changed persisted timestamp: first=%#v repeated=%#v", first, repeated)
	}
	if *first.LikedAt != "2026-07-27 12:01:02" {
		t.Fatalf("unexpected liked timestamp: %#v", first)
	}

	cleared, appErr := service.SetUserFeedback(context.Background(), 7, 44, false)
	if appErr != nil {
		t.Fatalf("unlike returned error: %v", appErr)
	}
	repeatedClear, appErr := service.SetUserFeedback(context.Background(), 7, 44, false)
	if appErr != nil {
		t.Fatalf("repeated unlike returned error: %v", appErr)
	}
	if cleared.Liked || cleared.LikedAt != nil || repeatedClear.Liked || repeatedClear.LikedAt != nil {
		t.Fatalf("unlike must stay cleared: first=%#v repeated=%#v", cleared, repeatedClear)
	}
	if clockReads != 1 {
		t.Fatalf("idempotent feedback must read the clock only for the first like, reads=%d", clockReads)
	}
	if len(repository.setRuns) != 2 || len(repository.setLike) != 2 || !repository.setLike[0] || repository.setLike[1] {
		t.Fatalf("unexpected feedback writes: runs=%v liked=%v", repository.setRuns, repository.setLike)
	}
}

func TestFeedbackRejectsForeignAndIneligibleRuns(t *testing.T) {
	conversationID := int64(3)
	assistantMessageID := int64(11)
	for _, test := range []struct {
		name       string
		run        *FeedbackRun
		statusCode int
		messageID  string
	}{
		{name: "missing", run: nil, statusCode: 404, messageID: "airun.feedback.not_found"},
		{name: "foreign owner", run: eligibleFeedbackRun(44, 8), statusCode: 404, messageID: "airun.feedback.not_found"},
		{name: "media run", run: &FeedbackRun{ID: 44, UserID: 7, Status: enum.AIRunStatusSuccess, AssistantMessageID: &assistantMessageID}, statusCode: 400, messageID: "airun.feedback.unavailable"},
		{name: "missing assistant message", run: &FeedbackRun{ID: 44, UserID: 7, Status: enum.AIRunStatusSuccess, ConversationID: &conversationID}, statusCode: 400, messageID: "airun.feedback.unavailable"},
		{name: "failed", run: feedbackRunWithStatus(44, 7, enum.AIRunStatusFailed), statusCode: 400, messageID: "airun.feedback.unavailable"},
		{name: "canceled", run: feedbackRunWithStatus(44, 7, enum.AIRunStatusCanceled), statusCode: 400, messageID: "airun.feedback.unavailable"},
		{name: "timeout", run: feedbackRunWithStatus(44, 7, enum.AIRunStatusTimeout), statusCode: 400, messageID: "airun.feedback.unavailable"},
		{name: "outcome unknown", run: feedbackRunWithStatus(44, 7, enum.AIRunStatusOutcomeUnknown), statusCode: 400, messageID: "airun.feedback.unavailable"},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository := &fakeFeedbackRepository{run: test.run}
			_, appErr := NewService(&fakeRepository{}, WithFeedbackRepository(repository)).SetUserFeedback(context.Background(), 7, 44, true)
			if appErr == nil || appErr.HTTPStatus != test.statusCode || appErr.MessageID != test.messageID {
				t.Fatalf("unexpected feedback rejection: %#v", appErr)
			}
			if len(repository.setRuns) != 0 {
				t.Fatalf("invalid feedback reached mutation: %v", repository.setRuns)
			}
		})
	}
}

func TestFeedbackRepositoryUsesOwnedRunQueryAndIdempotentLikedUpdate(t *testing.T) {
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{DisableAutomaticPing: true, Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	repository := &GormRepository{db: db}
	now := time.Date(2026, 7, 27, 12, 1, 2, 0, time.UTC)

	mock.ExpectQuery("SELECT id, user_id, conversation_id, status, assistant_message_id, liked_at FROM .*ai_runs.* WHERE id = \\? LIMIT \\?").
		WithArgs(int64(44), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "conversation_id", "status", "assistant_message_id", "liked_at"}).AddRow(44, 7, 3, "success", 11, nil))
	run, err := repository.FeedbackRun(context.Background(), 44)
	if err != nil || run == nil || run.UserID != 7 || run.ConversationID == nil || run.AssistantMessageID == nil {
		t.Fatalf("unexpected feedback run: row=%#v err=%v", run, err)
	}

	mock.ExpectBegin()
	mock.ExpectExec("UPDATE .*ai_runs.* SET .*liked_at.*COALESCE\\(liked_at, \\?\\).* WHERE id = \\? AND user_id = \\?").
		WithArgs(now, int64(44), int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectQuery("SELECT .*liked_at.* FROM .*ai_runs.* WHERE id = \\? AND user_id = \\? LIMIT \\?").
		WithArgs(int64(44), int64(7), 1).
		WillReturnRows(sqlmock.NewRows([]string{"liked_at"}).AddRow(now))
	likedAt, found, err := repository.UpdateUserFeedback(context.Background(), 44, 7, true, now)
	if err != nil || !found || likedAt == nil || !likedAt.Equal(now) {
		t.Fatalf("unexpected feedback update: likedAt=%v found=%v err=%v", likedAt, found, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("feedback repository query contract mismatch: %v", err)
	}
}

func eligibleFeedbackRun(id int64, userID int64) *FeedbackRun {
	return feedbackRunWithStatus(id, userID, enum.AIRunStatusSuccess)
}

func feedbackRunWithStatus(id int64, userID int64, status string) *FeedbackRun {
	conversationID := int64(3)
	assistantMessageID := int64(11)
	return &FeedbackRun{ID: id, UserID: userID, ConversationID: &conversationID, Status: status, AssistantMessageID: &assistantMessageID}
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
