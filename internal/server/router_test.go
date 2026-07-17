package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"admin_back_go/internal/config"
	infraai "admin_back_go/internal/infra/ai"
	infrarealtime "admin_back_go/internal/infra/realtime"
	"admin_back_go/internal/middleware"
	aiagent "admin_back_go/internal/module/ai/agent"
	aiasset "admin_back_go/internal/module/ai/asset"
	aiaudio "admin_back_go/internal/module/ai/audio"
	aichat "admin_back_go/internal/module/ai/chat"
	aiconversation "admin_back_go/internal/module/ai/conversation"
	aiimage "admin_back_go/internal/module/ai/image"
	aiknowledge "admin_back_go/internal/module/ai/knowledge"
	aimessage "admin_back_go/internal/module/ai/message"
	aiprompt "admin_back_go/internal/module/ai/prompt"
	aiprovider "admin_back_go/internal/module/ai/provider"
	airun "admin_back_go/internal/module/ai/run"
	aitool "admin_back_go/internal/module/ai/tool"
	aivideo "admin_back_go/internal/module/ai/video"
	"admin_back_go/internal/module/auth"
	authadmin "admin_back_go/internal/module/auth/transport/admin"
	"admin_back_go/internal/module/auth_platform"
	canvasmodule "admin_back_go/internal/module/canvas"
	"admin_back_go/internal/module/clientversion"
	"admin_back_go/internal/module/crontask"
	"admin_back_go/internal/module/export"
	"admin_back_go/internal/module/mail"
	"admin_back_go/internal/module/notification"
	notificationtask "admin_back_go/internal/module/notification/task"
	"admin_back_go/internal/module/operationlog"
	"admin_back_go/internal/module/payment"
	walletmodule "admin_back_go/internal/module/payment/wallet"
	"admin_back_go/internal/module/permission"
	"admin_back_go/internal/module/queuemonitor"
	realtimemodule "admin_back_go/internal/module/realtime"
	realtimeadmin "admin_back_go/internal/module/realtime/transport/admin"
	"admin_back_go/internal/module/role"
	"admin_back_go/internal/module/sms"
	"admin_back_go/internal/module/systemlog"
	"admin_back_go/internal/module/systemsetting"
	"admin_back_go/internal/module/uploadtoken"
	"admin_back_go/internal/module/user"
	"admin_back_go/internal/readiness"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/enum"
	"admin_back_go/internal/telemetry"

	"github.com/gorilla/websocket"
)

type routerBrowserGrantStore struct {
	mu     sync.Mutex
	values map[string]string
}

func (s *routerBrowserGrantStore) Put(_ context.Context, key string, value string, _ time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.values == nil {
		s.values = make(map[string]string)
	}
	s.values[key] = value
	return nil
}

func (s *routerBrowserGrantStore) Consume(_ context.Context, key string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value := s.values[key]
	delete(s.values, key)
	return value, nil
}

func (s *routerBrowserGrantStore) Get(_ context.Context, key string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.values[key], nil
}

func newRouterBrowserGrants() *auth.BrowserGrantService {
	return auth.NewBrowserGrantService(&routerBrowserGrantStore{}, auth.BrowserGrantConfig{RedisPrefix: "router-test:"})
}

type fakeReadinessChecker struct {
	report readiness.Report
}

func (f fakeReadinessChecker) Readiness(ctx context.Context) readiness.Report {
	return f.report
}

type fakeRouterAIKnowledgeService struct {
	initCalled             bool
	listQuery              aiknowledge.BaseListQuery
	detailID               uint64
	documentsBaseID        uint64
	createdDocumentBaseID  uint64
	documentDetailID       uint64
	documentUpdateID       uint64
	documentStatusID       uint64
	reindexDocumentID      uint64
	chunksDocumentID       uint64
	deletedDocumentID      uint64
	retrievalTestBaseID    uint64
	agentBindingsID        uint64
	updatedAgentBindingsID uint64
}

func (f *fakeRouterAIKnowledgeService) PageInit(ctx context.Context) (*aiknowledge.InitResponse, *apperror.Error) {
	f.initCalled = true
	return &aiknowledge.InitResponse{}, nil
}
func (f *fakeRouterAIKnowledgeService) ListBases(ctx context.Context, query aiknowledge.BaseListQuery) (*aiknowledge.BaseListResponse, *apperror.Error) {
	f.listQuery = query
	return &aiknowledge.BaseListResponse{List: []aiknowledge.BaseDTO{{ID: 1, Name: "架构库", Code: "arch", Status: enum.CommonYes}}, Page: aiknowledge.Page{CurrentPage: query.CurrentPage, PageSize: query.PageSize, Total: 1, TotalPage: 1}}, nil
}
func (f *fakeRouterAIKnowledgeService) GetBase(ctx context.Context, id uint64) (*aiknowledge.BaseDetailResponse, *apperror.Error) {
	f.detailID = id
	return &aiknowledge.BaseDetailResponse{BaseDTO: aiknowledge.BaseDTO{ID: id, Name: "架构库", Code: "arch", Status: enum.CommonYes}}, nil
}
func (f *fakeRouterAIKnowledgeService) CreateBase(ctx context.Context, input aiknowledge.BaseMutationInput) (uint64, *apperror.Error) {
	return 1, nil
}
func (f *fakeRouterAIKnowledgeService) UpdateBase(ctx context.Context, id uint64, input aiknowledge.BaseMutationInput) *apperror.Error {
	return nil
}
func (f *fakeRouterAIKnowledgeService) ChangeBaseStatus(ctx context.Context, id uint64, status int) *apperror.Error {
	return nil
}
func (f *fakeRouterAIKnowledgeService) DeleteBase(ctx context.Context, id uint64) *apperror.Error {
	return nil
}
func (f *fakeRouterAIKnowledgeService) ListDocuments(ctx context.Context, baseID uint64, query aiknowledge.DocumentListQuery) (*aiknowledge.DocumentListResponse, *apperror.Error) {
	f.documentsBaseID = baseID
	return &aiknowledge.DocumentListResponse{List: []aiknowledge.DocumentDTO{{ID: 2, KnowledgeBaseID: baseID, Title: "FAQ", SourceType: "text", Status: enum.CommonYes}}}, nil
}
func (f *fakeRouterAIKnowledgeService) GetDocument(ctx context.Context, id uint64) (*aiknowledge.DocumentDetailResponse, *apperror.Error) {
	f.documentDetailID = id
	return &aiknowledge.DocumentDetailResponse{DocumentDTO: aiknowledge.DocumentDTO{ID: id, Title: "FAQ", Status: enum.CommonYes}, Content: "hello"}, nil
}
func (f *fakeRouterAIKnowledgeService) CreateDocument(ctx context.Context, baseID uint64, input aiknowledge.DocumentMutationInput) (uint64, *apperror.Error) {
	f.createdDocumentBaseID = baseID
	return 2, nil
}
func (f *fakeRouterAIKnowledgeService) UpdateDocument(ctx context.Context, id uint64, input aiknowledge.DocumentMutationInput) *apperror.Error {
	f.documentUpdateID = id
	return nil
}
func (f *fakeRouterAIKnowledgeService) ChangeDocumentStatus(ctx context.Context, id uint64, status int) *apperror.Error {
	f.documentStatusID = id
	return nil
}
func (f *fakeRouterAIKnowledgeService) DeleteDocument(ctx context.Context, id uint64) *apperror.Error {
	f.deletedDocumentID = id
	return nil
}
func (f *fakeRouterAIKnowledgeService) ReindexDocument(ctx context.Context, id uint64) *apperror.Error {
	f.reindexDocumentID = id
	return nil
}
func (f *fakeRouterAIKnowledgeService) ListChunks(ctx context.Context, documentID uint64) (*aiknowledge.ChunkListResponse, *apperror.Error) {
	f.chunksDocumentID = documentID
	return &aiknowledge.ChunkListResponse{List: []aiknowledge.ChunkDTO{{ID: 3, DocumentID: documentID, ChunkIndex: 1}}}, nil
}
func (f *fakeRouterAIKnowledgeService) RetrievalTest(ctx context.Context, baseID uint64, input aiknowledge.RetrievalTestInput) (*aiknowledge.RetrievalResult, *apperror.Error) {
	f.retrievalTestBaseID = baseID
	return &aiknowledge.RetrievalResult{Query: input.Query, Status: aiknowledge.RetrievalStatusSuccess}, nil
}
func (f *fakeRouterAIKnowledgeService) AgentKnowledgeBases(ctx context.Context, agentID uint64) (*aiknowledge.AgentKnowledgeBindingsResponse, *apperror.Error) {
	f.agentBindingsID = agentID
	return &aiknowledge.AgentKnowledgeBindingsResponse{AgentID: agentID}, nil
}
func (f *fakeRouterAIKnowledgeService) UpdateAgentKnowledgeBases(ctx context.Context, agentID uint64, input aiknowledge.UpdateAgentKnowledgeBindingsInput) *apperror.Error {
	f.updatedAgentBindingsID = agentID
	return nil
}

type fakeRouterAIConversationService struct{}

func (fakeRouterAIConversationService) List(ctx context.Context, userID int64, query aiconversation.ListQuery) (*aiconversation.ListResponse, *apperror.Error) {
	return &aiconversation.ListResponse{List: []aiconversation.ConversationItem{{ID: 1, AgentID: 1, AgentName: "agent", Title: "会话"}}}, nil
}

func (fakeRouterAIConversationService) Detail(ctx context.Context, userID int64, id int64) (*aiconversation.ConversationDetail, *apperror.Error) {
	return &aiconversation.ConversationDetail{ID: id, AgentID: 1, AgentName: "agent", Title: "会话"}, nil
}

func (fakeRouterAIConversationService) Create(ctx context.Context, userID int64, input aiconversation.CreateInput) (int64, *apperror.Error) {
	return 1, nil
}

func (fakeRouterAIConversationService) Update(ctx context.Context, userID int64, id int64, input aiconversation.UpdateInput) *apperror.Error {
	return nil
}

func (fakeRouterAIConversationService) Delete(ctx context.Context, userID int64, id int64) *apperror.Error {
	return nil
}

type fakeRouterAIMessageService struct{}

func (fakeRouterAIMessageService) List(ctx context.Context, userID int64, query aimessage.ListQuery) (*aimessage.ListResponse, *apperror.Error) {
	return &aimessage.ListResponse{List: []aimessage.MessageItem{{ID: 2, Role: enum.AIMessageRoleUser, ContentType: "text", Content: "hello"}}}, nil
}

func (fakeRouterAIMessageService) Send(ctx context.Context, userID int64, input aimessage.SendInput) (*aimessage.SendResponse, *apperror.Error) {
	return &aimessage.SendResponse{ConversationID: input.ConversationID, UserMessageID: 2, RequestID: input.RequestID}, nil
}

func (fakeRouterAIMessageService) Cancel(ctx context.Context, userID int64, input aimessage.CancelInput) (*aimessage.CancelResponse, *apperror.Error) {
	return &aimessage.CancelResponse{ConversationID: input.ConversationID, RequestID: input.RequestID, Status: "canceled"}, nil
}

type fakeRouterAiImageService struct {
	createInput    aiimage.CreateInput
	editInput      aiimage.CreateWithUploadedFilesInput
	listUserID     uint64
	listQuery      aiimage.ListQuery
	detailUserID   uint64
	detailTaskID   uint64
	detailPlatform string
	deleteUserID   uint64
	deleteTaskID   uint64
	deletePlatform string
}

func (f *fakeRouterAiImageService) PageInit(ctx context.Context) (*aiimage.PageInitResponse, *apperror.Error) {
	return &aiimage.PageInitResponse{}, nil
}

func (f *fakeRouterAiImageService) List(ctx context.Context, userID uint64, query aiimage.ListQuery) (*aiimage.ListResponse, *apperror.Error) {
	f.listUserID = userID
	f.listQuery = query
	return &aiimage.ListResponse{List: []aiimage.TaskDTO{{ID: 88, Status: aiimage.StatusSuccess, Platform: enum.PlatformCanvas}}, Page: aiimage.Page{CurrentPage: query.CurrentPage, PageSize: query.PageSize, Total: 1, TotalPage: 1}}, nil
}

func (f *fakeRouterAiImageService) Detail(ctx context.Context, userID uint64, taskID uint64, platform string) (*aiimage.DetailResponse, *apperror.Error) {
	f.detailUserID = userID
	f.detailTaskID = taskID
	f.detailPlatform = platform
	return &aiimage.DetailResponse{Task: aiimage.TaskDTO{ID: taskID, Status: aiimage.StatusSuccess}}, nil
}

func (f *fakeRouterAiImageService) Create(ctx context.Context, input aiimage.CreateInput) (*aiimage.CreateTaskResponse, *apperror.Error) {
	f.createInput = input
	return &aiimage.CreateTaskResponse{Task: aiimage.TaskDTO{ID: 88, Status: aiimage.StatusPending}}, nil
}

func (f *fakeRouterAiImageService) CreateWithUploadedFiles(ctx context.Context, input aiimage.CreateWithUploadedFilesInput) (*aiimage.CreateTaskResponse, *apperror.Error) {
	f.editInput = input
	return &aiimage.CreateTaskResponse{Task: aiimage.TaskDTO{ID: 89, Status: aiimage.StatusPending}}, nil
}

func (f *fakeRouterAiImageService) Delete(ctx context.Context, userID uint64, taskID uint64, platform string) *apperror.Error {
	f.deleteUserID = userID
	f.deleteTaskID = taskID
	f.deletePlatform = platform
	return nil
}

type fakeRouterAIRunService struct{}

func (fakeRouterAIRunService) PageInit(ctx context.Context) (*airun.InitResponse, *apperror.Error) {
	return &airun.InitResponse{}, nil
}

func (fakeRouterAIRunService) List(ctx context.Context, query airun.ListQuery) (*airun.ListResponse, *apperror.Error) {
	return &airun.ListResponse{Page: airun.Page{CurrentPage: query.CurrentPage, PageSize: query.PageSize}}, nil
}

func (fakeRouterAIRunService) Detail(ctx context.Context, id int64) (*airun.DetailResponse, *apperror.Error) {
	return &airun.DetailResponse{ID: id}, nil
}

func (fakeRouterAIRunService) Stats(ctx context.Context, query airun.StatsFilter) (*airun.StatsResponse, *apperror.Error) {
	return &airun.StatsResponse{}, nil
}

func (fakeRouterAIRunService) StatsByDate(ctx context.Context, query airun.StatsListQuery) (*airun.StatsByDateResponse, *apperror.Error) {
	return &airun.StatsByDateResponse{Page: airun.Page{CurrentPage: query.CurrentPage, PageSize: query.PageSize}}, nil
}

func (fakeRouterAIRunService) StatsByAgent(ctx context.Context, query airun.StatsListQuery) (*airun.StatsByAgentResponse, *apperror.Error) {
	return &airun.StatsByAgentResponse{Page: airun.Page{CurrentPage: query.CurrentPage, PageSize: query.PageSize}}, nil
}

func (fakeRouterAIRunService) StatsByUser(ctx context.Context, query airun.StatsListQuery) (*airun.StatsByUserResponse, *apperror.Error) {
	return &airun.StatsByUserResponse{Page: airun.Page{CurrentPage: query.CurrentPage, PageSize: query.PageSize}}, nil
}

type fakeRouterAIChatService struct {
	input aichat.CanvasCompletionInput
}

func (f *fakeRouterAIChatService) CanvasCompletion(ctx context.Context, input aichat.CanvasCompletionInput) (*aichat.CanvasCompletionResponse, *apperror.Error) {
	f.input = input
	return &aichat.CanvasCompletionResponse{ID: "chat-1", Object: "chat.completion", Content: "ok"}, nil
}

type fakeRouterAIVideoService struct {
	createInput          aivideo.CreateInput
	referenceUploadInput aivideo.ReferenceMediaUploadInput
	statusUserID         int64
	statusID             int64
	contentUserID        int64
	contentID            int64
}

type fakeRouterAIAudioService struct {
	input aiaudio.GenerateInput
}

func (f *fakeRouterAIAudioService) Generate(ctx context.Context, input aiaudio.GenerateInput) (*aiaudio.GenerateResponse, *apperror.Error) {
	f.input = input
	return &aiaudio.GenerateResponse{Body: []byte("audio"), ContentType: "audio/mpeg"}, nil
}

func (f *fakeRouterAIVideoService) Create(ctx context.Context, input aivideo.CreateInput) (*aivideo.CreateResponse, *apperror.Error) {
	f.createInput = input
	return &aivideo.CreateResponse{ID: 99, Status: aivideo.StatusPending}, nil
}

func (f *fakeRouterAIVideoService) Status(ctx context.Context, userID int64, id int64) (*aivideo.StatusResponse, *apperror.Error) {
	f.statusUserID = userID
	f.statusID = id
	return &aivideo.StatusResponse{ID: id, Status: aivideo.StatusRunning}, nil
}

func (f *fakeRouterAIVideoService) Content(ctx context.Context, userID int64, id int64) ([]byte, string, *apperror.Error) {
	f.contentUserID = userID
	f.contentID = id
	return []byte("video"), "video/mp4", nil
}

func (f *fakeRouterAIVideoService) UploadReferenceMedia(ctx context.Context, input aivideo.ReferenceMediaUploadInput) (*aivideo.ReferenceMediaUploadResponse, *apperror.Error) {
	f.referenceUploadInput = input
	return &aivideo.ReferenceMediaUploadResponse{ID: "ref-1", URL: "https://cos.test/ref.mp4", StorageProvider: aivideo.StorageProviderCOS, StorageKey: "ai-video-references/video/ref.mp4", MimeType: input.MimeType, MediaKind: input.MediaKind, Bytes: int64(len(input.Body))}, nil
}

type fakeAppRouterAuthService struct {
	loginFn       func(context.Context, auth.LoginInput) (*auth.LoginResponse, *apperror.Error)
	loginConfigFn func(context.Context, string) (*auth.LoginConfigResponse, *apperror.Error)
	sendCodeFn    func(context.Context, auth.SendCodeInput) (string, *apperror.Error)
	logoutFn      func(context.Context, string) *apperror.Error
}

func (f *fakeAppRouterAuthService) Login(ctx context.Context, input auth.LoginInput) (*auth.LoginResponse, *apperror.Error) {
	if f.loginFn != nil {
		return f.loginFn(ctx, input)
	}
	return &auth.LoginResponse{AccessToken: "app-token"}, nil
}

func (f *fakeAppRouterAuthService) SendCode(ctx context.Context, input auth.SendCodeInput) (string, *apperror.Error) {
	if f.sendCodeFn != nil {
		return f.sendCodeFn(ctx, input)
	}
	return "", nil
}

func (f *fakeAppRouterAuthService) ForgetPassword(ctx context.Context, input auth.ForgetPasswordInput) *apperror.Error {
	return nil
}

func (f *fakeAppRouterAuthService) LoginConfig(ctx context.Context, platform string) (*auth.LoginConfigResponse, *apperror.Error) {
	if f.loginConfigFn != nil {
		return f.loginConfigFn(ctx, platform)
	}
	return &auth.LoginConfigResponse{}, nil
}

func (f *fakeAppRouterAuthService) Refresh(ctx context.Context, input auth.RefreshInput) (*auth.TokenResult, *apperror.Error) {
	return &auth.TokenResult{}, nil
}

func (f *fakeAppRouterAuthService) Logout(ctx context.Context, accessToken string) *apperror.Error {
	if f.logoutFn != nil {
		return f.logoutFn(ctx, accessToken)
	}
	return nil
}

type fakeAuthService struct{}

func (fakeAuthService) Login(ctx context.Context, input auth.LoginInput) (*auth.LoginResponse, *apperror.Error) {
	return &auth.LoginResponse{
		AccessToken:      "access-token",
		RefreshToken:     "refresh-token",
		ExpiresIn:        14400,
		RefreshExpiresIn: 1209600,
	}, nil
}

func (fakeAuthService) SendCode(ctx context.Context, input auth.SendCodeInput) (string, *apperror.Error) {
	return "验证码发送成功", nil
}

func (fakeAuthService) ForgetPassword(ctx context.Context, input auth.ForgetPasswordInput) *apperror.Error {
	return nil
}

func (fakeAuthService) LoginConfig(ctx context.Context, platform string) (*auth.LoginConfigResponse, *apperror.Error) {
	return &auth.LoginConfigResponse{
		LoginTypeArr:   []auth.LoginTypeOption{{Label: "密码登录", Value: auth.LoginTypePassword}},
		CaptchaEnabled: true,
		CaptchaType:    auth.TypeSlide,
	}, nil
}

func (fakeAuthService) Refresh(ctx context.Context, input auth.RefreshInput) (*auth.TokenResult, *apperror.Error) {
	return &auth.TokenResult{
		AccessToken:      "new-access",
		RefreshToken:     "new-refresh",
		ExpiresIn:        14400,
		RefreshExpiresIn: 1209600,
	}, nil
}

func (fakeAuthService) Logout(ctx context.Context, accessToken string) *apperror.Error {
	return nil
}

type fakeCaptchaService struct{}

func (fakeCaptchaService) Generate(ctx context.Context) (*auth.ChallengeResponse, *apperror.Error) {
	return &auth.ChallengeResponse{
		CaptchaID:   "captcha-id",
		CaptchaType: auth.TypeSlide,
		MasterImage: "data:image/jpeg;base64,master",
		TileImage:   "data:image/png;base64,tile",
		TileX:       7,
		TileY:       53,
		TileWidth:   62,
		TileHeight:  62,
		ImageWidth:  300,
		ImageHeight: 220,
		ExpiresIn:   120,
	}, nil
}

type fakeRouterUserService struct {
	input          user.InitInput
	result         *user.InitResponse
	err            *apperror.Error
	pageInitCalled bool
	profileUserID  int64
	profileViewer  int64
	profileResult  *user.ProfileResponse
	profileUpdate  user.UpdateProfileInput
	listQuery      user.ListQuery
	listResult     *user.ListResponse
	exportInput    user.ExportInput
}

type fakeRouterSessionAdminService struct {
	listQuery      auth.SessionListQuery
	revokeID       int64
	batchInput     auth.SessionBatchRevokeInput
	currentSession int64
}

func (fakeRouterSessionAdminService) PageInit(ctx context.Context) (*auth.SessionPageInitResponse, *apperror.Error) {
	return &auth.SessionPageInitResponse{}, nil
}

func (f *fakeRouterSessionAdminService) List(ctx context.Context, query auth.SessionListQuery) (*auth.SessionListResponse, *apperror.Error) {
	f.listQuery = query
	return &auth.SessionListResponse{
		List: []auth.SessionListItem{{ID: 1, UserID: 2, Username: "admin", Platform: "admin", Status: auth.SessionStatusActive}},
		Page: auth.SessionPage{CurrentPage: query.CurrentPage, PageSize: query.PageSize, Total: 1, TotalPage: 1},
	}, nil
}

func (fakeRouterSessionAdminService) Stats(ctx context.Context) (*auth.SessionStatsResponse, *apperror.Error) {
	return &auth.SessionStatsResponse{TotalActive: 0, PlatformDistribution: map[string]int64{"admin": 0, "app": 0}}, nil
}

func (f *fakeRouterSessionAdminService) Revoke(ctx context.Context, id int64, currentSessionID int64) (*auth.SessionRevokeResponse, *apperror.Error) {
	f.revokeID = id
	f.currentSession = currentSessionID
	return &auth.SessionRevokeResponse{ID: id, Revoked: true}, nil
}

func (f *fakeRouterSessionAdminService) BatchRevoke(ctx context.Context, input auth.SessionBatchRevokeInput, currentSessionID int64) (*auth.SessionBatchRevokeResponse, *apperror.Error) {
	f.batchInput = input
	f.currentSession = currentSessionID
	return &auth.SessionBatchRevokeResponse{Count: int64(len(input.IDs))}, nil
}

type fakeRouterLoginLogService struct {
	listQuery auth.LoginLogListQuery
}

func (fakeRouterLoginLogService) PageInit(ctx context.Context) (*auth.LoginLogPageInitResponse, *apperror.Error) {
	return &auth.LoginLogPageInitResponse{}, nil
}

func (f *fakeRouterLoginLogService) List(ctx context.Context, query auth.LoginLogListQuery) (*auth.LoginLogListResponse, *apperror.Error) {
	f.listQuery = query
	return &auth.LoginLogListResponse{
		List: []auth.LoginLogListItem{{ID: 1, UserName: "admin", LoginType: "password"}},
		Page: auth.LoginLogPage{CurrentPage: query.CurrentPage, PageSize: query.PageSize, Total: 1, TotalPage: 1},
	}, nil
}

func (f *fakeRouterUserService) Init(ctx context.Context, input user.InitInput) (*user.InitResponse, *apperror.Error) {
	f.input = input
	return f.result, f.err
}

func (f *fakeRouterUserService) PageInit(ctx context.Context) (*user.PageInitResponse, *apperror.Error) {
	f.pageInitCalled = true
	return &user.PageInitResponse{}, f.err
}

func (f *fakeRouterUserService) Profile(ctx context.Context, userID int64, currentUserID int64) (*user.ProfileResponse, *apperror.Error) {
	f.profileUserID = userID
	f.profileViewer = currentUserID
	if f.profileResult != nil {
		return f.profileResult, f.err
	}
	return &user.ProfileResponse{Profile: user.ProfileDetail{UserID: userID, Username: "admin"}}, f.err
}

func (f *fakeRouterUserService) UpdateProfile(ctx context.Context, input user.UpdateProfileInput) *apperror.Error {
	f.profileUserID = input.UserID
	f.profileUpdate = input
	return f.err
}

func (f *fakeRouterUserService) UpdatePassword(ctx context.Context, input user.UpdatePasswordInput) *apperror.Error {
	f.profileUserID = input.UserID
	return f.err
}

func (f *fakeRouterUserService) UpdateEmail(ctx context.Context, input user.UpdateEmailInput) *apperror.Error {
	f.profileUserID = input.UserID
	return f.err
}

func (f *fakeRouterUserService) UpdatePhone(ctx context.Context, input user.UpdatePhoneInput) *apperror.Error {
	f.profileUserID = input.UserID
	return f.err
}

func (f *fakeRouterUserService) List(ctx context.Context, query user.ListQuery) (*user.ListResponse, *apperror.Error) {
	f.listQuery = query
	if f.listResult != nil {
		return f.listResult, f.err
	}
	return &user.ListResponse{
		List: []user.ListItem{{ID: 1, Username: "admin"}},
		Page: user.Page{CurrentPage: query.CurrentPage, PageSize: query.PageSize, Total: 1, TotalPage: 1},
	}, f.err
}

func (f *fakeRouterUserService) Export(ctx context.Context, input user.ExportInput) (*user.ExportResponse, *apperror.Error) {
	f.exportInput = input
	return &user.ExportResponse{ID: 88, Message: "导出任务已提交，完成后将通知您"}, f.err
}

func (f *fakeRouterUserService) Update(ctx context.Context, id int64, input user.UpdateInput) *apperror.Error {
	return f.err
}

func (f *fakeRouterUserService) ChangeStatus(ctx context.Context, id int64, status int) *apperror.Error {
	return f.err
}

func (f *fakeRouterUserService) Delete(ctx context.Context, ids []int64) *apperror.Error {
	return f.err
}

func (f *fakeRouterUserService) BatchUpdateProfile(ctx context.Context, input user.BatchProfileUpdate) *apperror.Error {
	return f.err
}

type fakeRouterPermissionService struct {
	listQuery permission.PermissionListQuery
}

func (f *fakeRouterPermissionService) PageInit(ctx context.Context) (*permission.InitResponse, *apperror.Error) {
	return &permission.InitResponse{Dict: permission.PermissionDict{}}, nil
}

func (f *fakeRouterPermissionService) List(ctx context.Context, query permission.PermissionListQuery) ([]permission.PermissionListItem, *apperror.Error) {
	f.listQuery = query
	return []permission.PermissionListItem{{ID: 1, Name: "系统"}}, nil
}

func (f *fakeRouterPermissionService) Create(ctx context.Context, input permission.PermissionMutationInput) (int64, *apperror.Error) {
	return 1, nil
}

func (f *fakeRouterPermissionService) Update(ctx context.Context, id int64, input permission.PermissionMutationInput) *apperror.Error {
	return nil
}

func (f *fakeRouterPermissionService) Delete(ctx context.Context, ids []int64) *apperror.Error {
	return nil
}

func (f *fakeRouterPermissionService) ChangeStatus(ctx context.Context, id int64, status int) *apperror.Error {
	return nil
}

type fakeRouterRoleService struct {
	listQuery role.ListQuery
}

func (f *fakeRouterRoleService) PageInit(ctx context.Context) (*role.InitResponse, *apperror.Error) {
	return &role.InitResponse{}, nil
}

func (f *fakeRouterRoleService) List(ctx context.Context, query role.ListQuery) (*role.ListResponse, *apperror.Error) {
	f.listQuery = query
	return &role.ListResponse{
		List: []role.ListItem{{ID: 1, Name: "管理员"}},
		Page: role.Page{CurrentPage: query.CurrentPage, PageSize: query.PageSize, Total: 1, TotalPage: 1},
	}, nil
}

func (f *fakeRouterRoleService) Create(ctx context.Context, input role.MutationInput) (int64, *apperror.Error) {
	return 1, nil
}

func (f *fakeRouterRoleService) Update(ctx context.Context, id int64, input role.MutationInput) *apperror.Error {
	return nil
}

func (f *fakeRouterRoleService) Delete(ctx context.Context, ids []int64) *apperror.Error {
	return nil
}

func (f *fakeRouterRoleService) SetDefault(ctx context.Context, id int64) *apperror.Error {
	return nil
}

type fakeRouterAuthPlatformService struct {
	listQuery authplatform.ListQuery
}

func (f *fakeRouterAuthPlatformService) PageInit(ctx context.Context) (*authplatform.InitResponse, *apperror.Error) {
	return (&authplatform.Service{}).PageInit(ctx)
}

func (f *fakeRouterAuthPlatformService) List(ctx context.Context, query authplatform.ListQuery) (*authplatform.ListResponse, *apperror.Error) {
	f.listQuery = query
	return &authplatform.ListResponse{
		List: []authplatform.ListItem{{ID: 1, Code: "admin", Name: "PC后台", CaptchaType: auth.TypeSlide}},
		Page: authplatform.Page{CurrentPage: query.CurrentPage, PageSize: query.PageSize, Total: 1, TotalPage: 1},
	}, nil
}

func (f *fakeRouterAuthPlatformService) Create(ctx context.Context, input authplatform.CreateInput) (int64, *apperror.Error) {
	return 1, nil
}

func (f *fakeRouterAuthPlatformService) Update(ctx context.Context, id int64, input authplatform.UpdateInput) *apperror.Error {
	return nil
}

func (f *fakeRouterAuthPlatformService) Delete(ctx context.Context, ids []int64) *apperror.Error {
	return nil
}

func (f *fakeRouterAuthPlatformService) ChangeStatus(ctx context.Context, id int64, status int) *apperror.Error {
	return nil
}

type fakeRouterClientVersionService struct {
	initCalled         bool
	listQuery          clientversion.ListQuery
	createInput        clientversion.CreateInput
	updateID           int64
	updateInput        clientversion.UpdateInput
	latestID           int64
	forceID            int64
	forceUpdate        int
	deleteID           int64
	updateJSONPlatform string
	currentCheckQuery  clientversion.CurrentCheckQuery
}

func (f *fakeRouterClientVersionService) PageInit(ctx context.Context) (*clientversion.InitResponse, *apperror.Error) {
	f.initCalled = true
	return &clientversion.InitResponse{}, nil
}

func (f *fakeRouterClientVersionService) List(ctx context.Context, query clientversion.ListQuery) (*clientversion.ListResponse, *apperror.Error) {
	f.listQuery = query
	return &clientversion.ListResponse{
		List: []clientversion.ListItem{{ID: 8, Version: "1.0.7", Platform: enum.ClientPlatformWindowsX8664}},
		Page: clientversion.Page{CurrentPage: query.CurrentPage, PageSize: query.PageSize, Total: 1, TotalPage: 1},
	}, nil
}

func (f *fakeRouterClientVersionService) Create(ctx context.Context, input clientversion.CreateInput) (int64, *apperror.Error) {
	f.createInput = input
	return 9, nil
}

func (f *fakeRouterClientVersionService) Update(ctx context.Context, id int64, input clientversion.UpdateInput) *apperror.Error {
	f.updateID = id
	f.updateInput = input
	return nil
}

func (f *fakeRouterClientVersionService) SetLatest(ctx context.Context, id int64) *apperror.Error {
	f.latestID = id
	return nil
}

func (f *fakeRouterClientVersionService) ForceUpdate(ctx context.Context, id int64, forceUpdate int) *apperror.Error {
	f.forceID = id
	f.forceUpdate = forceUpdate
	return nil
}

func (f *fakeRouterClientVersionService) Delete(ctx context.Context, id int64) *apperror.Error {
	f.deleteID = id
	return nil
}

func (f *fakeRouterClientVersionService) UpdateJSON(ctx context.Context, platform string) (any, *apperror.Error) {
	f.updateJSONPlatform = platform
	return clientversion.ManifestPayload{
		Version: "1.0.7",
		Platforms: map[string]clientversion.ManifestPlatform{
			platform: {URL: "https://example.com/app.exe", Signature: "sig"},
		},
	}, nil
}

func (f *fakeRouterClientVersionService) CurrentCheck(ctx context.Context, query clientversion.CurrentCheckQuery) (*clientversion.CurrentCheckResponse, *apperror.Error) {
	f.currentCheckQuery = query
	return &clientversion.CurrentCheckResponse{ForceUpdate: true}, nil
}

type fakeRouterAIProviderService struct {
	initCalled       bool
	listQuery        aiprovider.ListQuery
	testID           uint64
	previewCalled    bool
	storedPreviewID  uint64
	syncID           uint64
	modelsID         uint64
	updateModelsID   uint64
	updateModelsBody aiprovider.UpdateModelsInput
}

func (f *fakeRouterAIProviderService) PageInit(ctx context.Context) (*aiprovider.InitResponse, *apperror.Error) {
	f.initCalled = true
	return &aiprovider.InitResponse{}, nil
}

func (f *fakeRouterAIProviderService) List(ctx context.Context, query aiprovider.ListQuery) (*aiprovider.ListResponse, *apperror.Error) {
	f.listQuery = query
	return &aiprovider.ListResponse{
		List: []aiprovider.ProviderDTO{{ID: 1, Name: "OpenAI", EngineType: "openai", APIKeyMasked: "***test", Status: enum.CommonYes}},
		Page: aiprovider.Page{CurrentPage: query.CurrentPage, PageSize: query.PageSize, Total: 1, TotalPage: 1},
	}, nil
}

func (f *fakeRouterAIProviderService) Create(ctx context.Context, input aiprovider.CreateInput) (uint64, *apperror.Error) {
	return 1, nil
}

func (f *fakeRouterAIProviderService) Update(ctx context.Context, id uint64, input aiprovider.UpdateInput) *apperror.Error {
	return nil
}

func (f *fakeRouterAIProviderService) ChangeStatus(ctx context.Context, id uint64, status int) *apperror.Error {
	return nil
}

func (f *fakeRouterAIProviderService) TestConnection(ctx context.Context, id uint64) (*infraai.TestConnectionResult, *apperror.Error) {
	f.testID = id
	return &infraai.TestConnectionResult{OK: true, Status: "200 OK", Message: "ok"}, nil
}

func (f *fakeRouterAIProviderService) PreviewModels(ctx context.Context, input aiprovider.ModelOptionsInput) (*aiprovider.ModelOptionsResponse, *apperror.Error) {
	f.previewCalled = true
	return &aiprovider.ModelOptionsResponse{}, nil
}

func (f *fakeRouterAIProviderService) PreviewStoredModels(ctx context.Context, id uint64) (*aiprovider.ModelOptionsResponse, *apperror.Error) {
	f.storedPreviewID = id
	return &aiprovider.ModelOptionsResponse{}, nil
}

func (f *fakeRouterAIProviderService) SyncModels(ctx context.Context, id uint64) (*aiprovider.ModelOptionsResponse, *apperror.Error) {
	f.syncID = id
	return &aiprovider.ModelOptionsResponse{}, nil
}

func (f *fakeRouterAIProviderService) ListProviderModels(ctx context.Context, id uint64) (*aiprovider.ProviderModelsResponse, *apperror.Error) {
	f.modelsID = id
	return &aiprovider.ProviderModelsResponse{}, nil
}

func (f *fakeRouterAIProviderService) UpdateProviderModels(ctx context.Context, id uint64, input aiprovider.UpdateModelsInput) *apperror.Error {
	f.updateModelsID = id
	f.updateModelsBody = input
	return nil
}

func (f *fakeRouterAIProviderService) Delete(ctx context.Context, id uint64) *apperror.Error {
	return nil
}

type fakeRouterAIAgentService struct {
	initCalled       bool
	listQuery        aiagent.ListQuery
	providerModelsID uint64
	detailID         uint64
	testID           uint64
	optionQuery      aiagent.OptionQuery
}

func (f *fakeRouterAIAgentService) PageInit(ctx context.Context) (*aiagent.InitResponse, *apperror.Error) {
	f.initCalled = true
	return &aiagent.InitResponse{}, nil
}

func (f *fakeRouterAIAgentService) List(ctx context.Context, query aiagent.ListQuery) (*aiagent.ListResponse, *apperror.Error) {
	f.listQuery = query
	return &aiagent.ListResponse{
		List: []aiagent.AgentDTO{{ID: 1, Name: "客服助手", Status: enum.CommonYes}},
		Page: aiagent.Page{CurrentPage: query.CurrentPage, PageSize: query.PageSize, Total: 1, TotalPage: 1},
	}, nil
}

func (f *fakeRouterAIAgentService) ProviderModels(ctx context.Context, providerID uint64) (*aiagent.ProviderModelsResponse, *apperror.Error) {
	f.providerModelsID = providerID
	return &aiagent.ProviderModelsResponse{List: []aiagent.ProviderModelDTO{{ProviderID: providerID, ModelID: "gpt-4.1-mini", DisplayName: "GPT-4.1 mini", Status: enum.CommonYes}}}, nil
}

func (f *fakeRouterAIAgentService) Detail(ctx context.Context, id uint64) (*aiagent.DetailResponse, *apperror.Error) {
	f.detailID = id
	return &aiagent.DetailResponse{AgentDTO: aiagent.AgentDTO{ID: id, Name: "客服助手", Status: enum.CommonYes}}, nil
}

func (f *fakeRouterAIAgentService) Create(ctx context.Context, input aiagent.CreateInput) (uint64, *apperror.Error) {
	return 1, nil
}

func (f *fakeRouterAIAgentService) Update(ctx context.Context, id uint64, input aiagent.UpdateInput) *apperror.Error {
	return nil
}

func (f *fakeRouterAIAgentService) ChangeStatus(ctx context.Context, id uint64, status int) *apperror.Error {
	return nil
}

func (f *fakeRouterAIAgentService) Test(ctx context.Context, id uint64) (*infraai.TestConnectionResult, *apperror.Error) {
	f.testID = id
	return &infraai.TestConnectionResult{OK: true, Status: "200 OK", Message: "ok"}, nil
}

func (f *fakeRouterAIAgentService) Delete(ctx context.Context, id uint64) *apperror.Error {
	return nil
}

func (f *fakeRouterAIAgentService) Options(ctx context.Context, query aiagent.OptionQuery) (*aiagent.AgentOptionsResponse, *apperror.Error) {
	f.optionQuery = query
	return &aiagent.AgentOptionsResponse{List: []aiagent.AgentOption{{ID: 1, Name: "客服助手"}}}, nil
}

type fakeRouterAIToolService struct {
	initCalled    bool
	listQuery     aitool.ListQuery
	updatedID     uint64
	statusID      uint64
	deletedID     uint64
	bindingID     uint64
	bindingToolID []uint64
	generateInit  bool
	generateInput aitool.GenerateDraftInput
}

func (f *fakeRouterAIToolService) PageInit(ctx context.Context) (*aitool.InitResponse, *apperror.Error) {
	f.initCalled = true
	return &aitool.InitResponse{}, nil
}

func (f *fakeRouterAIToolService) List(ctx context.Context, query aitool.ListQuery) (*aitool.ListResponse, *apperror.Error) {
	f.listQuery = query
	return &aitool.ListResponse{
		List: []aitool.ToolDTO{{ID: 1, Name: "查询当前用户量", Code: "admin_user_count", RiskLevel: aitool.RiskLow, Status: enum.CommonYes}},
		Page: aitool.Page{CurrentPage: query.CurrentPage, PageSize: query.PageSize, Total: 1, TotalPage: 1},
	}, nil
}

func (f *fakeRouterAIToolService) GeneratePageInit(ctx context.Context) (*aitool.GeneratePageInitResponse, *apperror.Error) {
	f.generateInit = true
	return &aitool.GeneratePageInitResponse{AgentOptions: []aitool.GenerateAgentOption{{Label: "工具生成", Value: 5}}}, nil
}

func (f *fakeRouterAIToolService) GenerateDraft(ctx context.Context, input aitool.GenerateDraftInput) (*aitool.GenerateDraftResponse, *apperror.Error) {
	f.generateInput = input
	return &aitool.GenerateDraftResponse{
		OK: true,
		Draft: &aitool.GeneratedToolDraft{
			Name:             "查询当前用户量",
			Code:             "admin_user_count",
			Description:      "查询数量",
			ParametersJSON:   json.RawMessage(`{"type":"object","properties":{},"required":[],"additionalProperties":false}`),
			ResultSchemaJSON: json.RawMessage(`{"type":"object","properties":{},"required":[],"additionalProperties":false}`),
			RiskLevel:        aitool.RiskLow,
			TimeoutMS:        3000,
			Status:           enum.CommonYes,
		},
		Warnings:            []string{},
		ClarifyingQuestions: []string{},
	}, nil
}

func (f *fakeRouterAIToolService) Create(ctx context.Context, input aitool.MutationInput) (uint64, *apperror.Error) {
	return 1, nil
}

func (f *fakeRouterAIToolService) Update(ctx context.Context, id uint64, input aitool.MutationInput) *apperror.Error {
	f.updatedID = id
	return nil
}

func (f *fakeRouterAIToolService) ChangeStatus(ctx context.Context, id uint64, status int) *apperror.Error {
	f.statusID = id
	return nil
}

func (f *fakeRouterAIToolService) Delete(ctx context.Context, id uint64) *apperror.Error {
	f.deletedID = id
	return nil
}

func (f *fakeRouterAIToolService) AgentTools(ctx context.Context, agentID uint64) (*aitool.AgentToolsResponse, *apperror.Error) {
	f.bindingID = agentID
	return &aitool.AgentToolsResponse{AgentID: agentID, ToolIDs: []uint64{1}, ActiveToolIDs: []uint64{1}}, nil
}

func (f *fakeRouterAIToolService) UpdateAgentTools(ctx context.Context, agentID uint64, input aitool.UpdateAgentToolsInput) *apperror.Error {
	f.bindingID = agentID
	f.bindingToolID = append([]uint64(nil), input.ToolIDs...)
	return nil
}

type fakeRouterOperationLogService struct {
	initCalled bool
	listQuery  operationlog.ListQuery
	deleteIDs  []int64
	listResult *operationlog.ListResponse
}

func (f *fakeRouterOperationLogService) PageInit(ctx context.Context) (*operationlog.InitResponse, *apperror.Error) {
	f.initCalled = true
	return &operationlog.InitResponse{}, nil
}

func (f *fakeRouterOperationLogService) List(ctx context.Context, query operationlog.ListQuery) (*operationlog.ListResponse, *apperror.Error) {
	f.listQuery = query
	if f.listResult != nil {
		return f.listResult, nil
	}
	return &operationlog.ListResponse{
		Page: operationlog.Page{CurrentPage: query.CurrentPage, PageSize: query.PageSize, Total: 1, TotalPage: 1},
	}, nil
}

func (f *fakeRouterOperationLogService) Delete(ctx context.Context, ids []int64) *apperror.Error {
	f.deleteIDs = ids
	return nil
}

type fakeRouterNotificationService struct {
	listQuery      notification.ListQuery
	unreadIdentity notification.Identity
	markIdentity   notification.Identity
	markIDs        []int64
	deleteIdentity notification.Identity
	deleteIDs      []int64
}

func (f *fakeRouterNotificationService) PageInit(ctx context.Context) (*notification.InitResponse, *apperror.Error) {
	return notification.NewService(&fakeRepositoryForNotificationRouter{}).PageInit(ctx)
}

func (f *fakeRouterNotificationService) List(ctx context.Context, query notification.ListQuery) (*notification.ListResponse, *apperror.Error) {
	f.listQuery = query
	return &notification.ListResponse{
		List: []notification.ListItem{{ID: 1, Title: "通知"}},
		Page: notification.Page{CurrentPage: query.CurrentPage, PageSize: query.PageSize, Total: 1, TotalPage: 1},
	}, nil
}

func (f *fakeRouterNotificationService) UnreadCount(ctx context.Context, identity notification.Identity) (*notification.UnreadCountResponse, *apperror.Error) {
	f.unreadIdentity = identity
	return &notification.UnreadCountResponse{Count: 2}, nil
}

func (f *fakeRouterNotificationService) MarkRead(ctx context.Context, identity notification.Identity, ids []int64) *apperror.Error {
	f.markIdentity = identity
	f.markIDs = append([]int64{}, ids...)
	return nil
}

func (f *fakeRouterNotificationService) Delete(ctx context.Context, identity notification.Identity, ids []int64) *apperror.Error {
	f.deleteIdentity = identity
	f.deleteIDs = append([]int64{}, ids...)
	return nil
}

type fakeRouterNotificationTaskService struct {
	statusCountQuery notificationtask.StatusCountQuery
	listQuery        notificationtask.ListQuery
	createInput      notificationtask.CreateInput
	cancelID         int64
	deleteID         int64
}

func (f *fakeRouterNotificationTaskService) PageInit(ctx context.Context) (*notificationtask.InitResponse, *apperror.Error) {
	return notificationtask.NewService(&fakeRepositoryForNotificationTaskRouter{}).PageInit(ctx)
}

func (f *fakeRouterNotificationTaskService) StatusCount(ctx context.Context, query notificationtask.StatusCountQuery) ([]notificationtask.StatusCountItem, *apperror.Error) {
	f.statusCountQuery = query
	return []notificationtask.StatusCountItem{{Label: "待发送", Value: 1, Num: 2}}, nil
}

func (f *fakeRouterNotificationTaskService) List(ctx context.Context, query notificationtask.ListQuery) (*notificationtask.ListResponse, *apperror.Error) {
	f.listQuery = query
	return &notificationtask.ListResponse{
		List: []notificationtask.ListItem{{ID: 1, Title: "发布通知"}},
		Page: notificationtask.Page{CurrentPage: query.CurrentPage, PageSize: query.PageSize, Total: 1, TotalPage: 1},
	}, nil
}

func (f *fakeRouterNotificationTaskService) Create(ctx context.Context, input notificationtask.CreateInput) (*notificationtask.CreateResponse, *apperror.Error) {
	f.createInput = input
	return &notificationtask.CreateResponse{ID: 7, Queued: true}, nil
}

func (f *fakeRouterNotificationTaskService) Cancel(ctx context.Context, id int64) *apperror.Error {
	f.cancelID = id
	return nil
}

func (f *fakeRouterNotificationTaskService) Delete(ctx context.Context, id int64) *apperror.Error {
	f.deleteID = id
	return nil
}

type fakeRouterExportTaskService struct {
	statusQuery exporttask.StatusCountQuery
	listQuery   exporttask.ListQuery
	deleteInput exporttask.DeleteInput
}

func (f *fakeRouterExportTaskService) StatusCount(ctx context.Context, query exporttask.StatusCountQuery) ([]exporttask.StatusCountItem, *apperror.Error) {
	f.statusQuery = query
	return []exporttask.StatusCountItem{{Label: "处理中", Value: 1, Num: 1}}, nil
}

func (f *fakeRouterExportTaskService) List(ctx context.Context, query exporttask.ListQuery) (*exporttask.ListResponse, *apperror.Error) {
	f.listQuery = query
	return &exporttask.ListResponse{List: []exporttask.ListItem{}, Page: exporttask.Page{CurrentPage: query.CurrentPage, PageSize: query.PageSize}}, nil
}

func (f *fakeRouterExportTaskService) Delete(ctx context.Context, input exporttask.DeleteInput) *apperror.Error {
	f.deleteInput = input
	return nil
}

type fakeRepositoryForNotificationTaskRouter struct{}

func (fakeRepositoryForNotificationTaskRouter) List(ctx context.Context, query notificationtask.ListQuery) ([]notificationtask.Task, int64, error) {
	return nil, 0, nil
}

func (fakeRepositoryForNotificationTaskRouter) CountByStatus(ctx context.Context, query notificationtask.StatusCountQuery) (map[int]int64, error) {
	return nil, nil
}

func (fakeRepositoryForNotificationTaskRouter) Create(ctx context.Context, row notificationtask.Task) (int64, error) {
	return 0, nil
}

func (fakeRepositoryForNotificationTaskRouter) Get(ctx context.Context, id int64) (*notificationtask.Task, error) {
	return nil, nil
}

func (fakeRepositoryForNotificationTaskRouter) CancelPending(ctx context.Context, id int64) (int64, error) {
	return 0, nil
}

func (fakeRepositoryForNotificationTaskRouter) Delete(ctx context.Context, id int64) (int64, error) {
	return 0, nil
}

func (fakeRepositoryForNotificationTaskRouter) CountTargetUsers(ctx context.Context, targetType int, targetIDs []int64) (int, error) {
	return 0, nil
}

func (fakeRepositoryForNotificationTaskRouter) ClaimDueTasks(ctx context.Context, now time.Time, limit int) ([]int64, error) {
	return nil, nil
}

func (fakeRepositoryForNotificationTaskRouter) ClaimSendTask(ctx context.Context, id int64) (*notificationtask.Task, bool, error) {
	return nil, false, nil
}

func (fakeRepositoryForNotificationTaskRouter) TargetUserIDs(ctx context.Context, task notificationtask.Task) ([]int64, error) {
	return nil, nil
}

func (fakeRepositoryForNotificationTaskRouter) InsertNotifications(ctx context.Context, rows []notificationtask.Notification) error {
	return nil
}

func (fakeRepositoryForNotificationTaskRouter) UpdateProgress(ctx context.Context, id int64, sentCount int, totalCount int) error {
	return nil
}

func (fakeRepositoryForNotificationTaskRouter) MarkSuccess(ctx context.Context, id int64, sentCount int, totalCount int) error {
	return nil
}

func (fakeRepositoryForNotificationTaskRouter) MarkFailed(ctx context.Context, id int64, errMsg string) error {
	return nil
}

type fakeRepositoryForNotificationRouter struct{}

func (fakeRepositoryForNotificationRouter) List(ctx context.Context, query notification.ListQuery) ([]notification.Notification, int64, error) {
	return nil, 0, nil
}

func (fakeRepositoryForNotificationRouter) UnreadCount(ctx context.Context, userID int64, platform string) (int64, error) {
	return 0, nil
}

func (fakeRepositoryForNotificationRouter) MarkRead(ctx context.Context, input notification.MarkReadInput) (int64, error) {
	return 0, nil
}

func (fakeRepositoryForNotificationRouter) Delete(ctx context.Context, input notification.DeleteInput) (int64, error) {
	return 0, nil
}

type fakeRouterCronTaskService struct {
	listQuery crontask.ListQuery
	statusID  int64
	status    int
	logsQuery crontask.LogsQuery
}

func (f *fakeRouterCronTaskService) PageInit(ctx context.Context) (*crontask.InitResponse, *apperror.Error) {
	return crontask.NewService(&fakeCronTaskRepositoryForRouter{}, crontask.NewDefaultRegistry()).PageInit(ctx)
}

func (f *fakeRouterCronTaskService) List(ctx context.Context, query crontask.ListQuery) (*crontask.ListResponse, *apperror.Error) {
	f.listQuery = query
	return &crontask.ListResponse{
		List: []crontask.ListItem{{ID: 1, Name: "notification_task_scheduler", Handler: "notification:dispatch-due:v1"}},
		Page: crontask.Page{CurrentPage: query.CurrentPage, PageSize: query.PageSize, Total: 1, TotalPage: 1},
	}, nil
}

func (f *fakeRouterCronTaskService) Create(ctx context.Context, input crontask.SaveInput) (*crontask.ListItem, *apperror.Error) {
	return &crontask.ListItem{ID: 1, Name: input.Name, Title: input.Title}, nil
}

func (f *fakeRouterCronTaskService) Update(ctx context.Context, id int64, input crontask.SaveInput) *apperror.Error {
	return nil
}

func (f *fakeRouterCronTaskService) ChangeStatus(ctx context.Context, id int64, status int) *apperror.Error {
	f.statusID = id
	f.status = status
	return nil
}

func (f *fakeRouterCronTaskService) Delete(ctx context.Context, ids []int64) *apperror.Error {
	return nil
}

func (f *fakeRouterCronTaskService) Logs(ctx context.Context, query crontask.LogsQuery) (*crontask.LogsResponse, *apperror.Error) {
	f.logsQuery = query
	return &crontask.LogsResponse{List: []crontask.LogItem{}, Page: crontask.Page{CurrentPage: query.CurrentPage, PageSize: query.PageSize}}, nil
}

type fakeCronTaskRepositoryForRouter struct{}

func (fakeCronTaskRepositoryForRouter) List(ctx context.Context, query crontask.ListQuery) ([]crontask.Task, int64, error) {
	return nil, 0, nil
}
func (fakeCronTaskRepositoryForRouter) NameExists(ctx context.Context, name string, excludeID int64) (bool, error) {
	return false, nil
}
func (fakeCronTaskRepositoryForRouter) Create(ctx context.Context, row crontask.Task) (int64, error) {
	return 0, nil
}
func (fakeCronTaskRepositoryForRouter) Get(ctx context.Context, id int64) (*crontask.Task, error) {
	return nil, crontask.ErrTaskNotFound
}
func (fakeCronTaskRepositoryForRouter) Update(ctx context.Context, id int64, row crontask.Task) error {
	return nil
}
func (fakeCronTaskRepositoryForRouter) UpdateStatus(ctx context.Context, id int64, status int) error {
	return nil
}
func (fakeCronTaskRepositoryForRouter) Delete(ctx context.Context, ids []int64) error { return nil }
func (fakeCronTaskRepositoryForRouter) Logs(ctx context.Context, query crontask.LogsQuery) ([]crontask.TaskLog, int64, error) {
	return nil, 0, nil
}
func (fakeCronTaskRepositoryForRouter) ListEnabled(ctx context.Context) ([]crontask.Task, error) {
	return nil, nil
}
func (fakeCronTaskRepositoryForRouter) LogStart(ctx context.Context, task crontask.Task, now time.Time) (int64, error) {
	return 0, nil
}
func (fakeCronTaskRepositoryForRouter) LogEnd(ctx context.Context, logID int64, success bool, result string, errMsg string, now time.Time) error {
	return nil
}

type fakeRouterSystemLogService struct {
	filesCalled bool
	linesQuery  systemlog.LinesQuery
}

func (f *fakeRouterSystemLogService) PageInit(ctx context.Context) (*systemlog.InitResponse, *apperror.Error) {
	return systemlog.NewService(nil).PageInit(ctx)
}

func (f *fakeRouterSystemLogService) Files(ctx context.Context) (*systemlog.FilesResponse, *apperror.Error) {
	f.filesCalled = true
	return &systemlog.FilesResponse{List: []systemlog.FileItem{{Name: "admin-api.log", Size: 1, SizeHuman: "1 B", MTime: "2026-05-04 10:00:00"}}}, nil
}

func (f *fakeRouterSystemLogService) Lines(ctx context.Context, query systemlog.LinesQuery) (*systemlog.LinesResponse, *apperror.Error) {
	f.linesQuery = query
	return &systemlog.LinesResponse{Filename: query.Filename, Total: 1, Lines: []systemlog.LineItem{{Number: 1, Level: "ERROR", Content: "ERROR boom"}}}, nil
}

type fakeRouterSystemSettingService struct {
	listQuery systemsetting.ListQuery
	statusID  int64
	status    int
}

func (f *fakeRouterSystemSettingService) PageInit(ctx context.Context) (*systemsetting.InitResponse, *apperror.Error) {
	return systemsetting.NewService(nil).PageInit(ctx)
}

func (f *fakeRouterSystemSettingService) List(ctx context.Context, query systemsetting.ListQuery) (*systemsetting.ListResponse, *apperror.Error) {
	f.listQuery = query
	return &systemsetting.ListResponse{
		List: []systemsetting.ListItem{{ID: 1, SettingKey: "user.default_avatar"}},
		Page: systemsetting.Page{CurrentPage: query.CurrentPage, PageSize: query.PageSize, Total: 1, TotalPage: 1},
	}, nil
}

func (f *fakeRouterSystemSettingService) Create(ctx context.Context, input systemsetting.CreateInput) (int64, *apperror.Error) {
	return 1, nil
}

func (f *fakeRouterSystemSettingService) Update(ctx context.Context, id int64, input systemsetting.UpdateInput) *apperror.Error {
	return nil
}

func (f *fakeRouterSystemSettingService) Delete(ctx context.Context, ids []int64) *apperror.Error {
	return nil
}

func (f *fakeRouterSystemSettingService) ChangeStatus(ctx context.Context, id int64, status int) *apperror.Error {
	f.statusID = id
	f.status = status
	return nil
}

type fakeRouterMailService struct {
	initCalled  bool
	savedConfig mail.SaveConfigInput
	deletedLogs []uint64
}

func (f *fakeRouterMailService) PageInit(ctx context.Context) (*mail.PageInitResponse, *apperror.Error) {
	f.initCalled = true
	return &mail.PageInitResponse{Dict: mail.PageInitDict{DefaultRegion: mail.DefaultRegion, DefaultEndpoint: mail.DefaultEndpoint}}, nil
}

func (f *fakeRouterMailService) Config(ctx context.Context) (*mail.ConfigResponse, *apperror.Error) {
	return &mail.ConfigResponse{Configured: true, Region: mail.DefaultRegion, Endpoint: mail.DefaultEndpoint, FromEmail: "noreply@example.com"}, nil
}

func (f *fakeRouterMailService) SaveConfig(ctx context.Context, input mail.SaveConfigInput) *apperror.Error {
	f.savedConfig = input
	return nil
}

func (f *fakeRouterMailService) DeleteConfig(ctx context.Context) *apperror.Error { return nil }
func (f *fakeRouterMailService) TestSend(ctx context.Context, input mail.TestInput) *apperror.Error {
	return nil
}
func (f *fakeRouterMailService) Templates(ctx context.Context) ([]mail.TemplateDTO, *apperror.Error) {
	return []mail.TemplateDTO{{ID: 1, Scene: enum.VerifyCodeSceneLogin, Name: "登录验证码"}}, nil
}
func (f *fakeRouterMailService) CreateTemplate(ctx context.Context, input mail.SaveTemplateInput) (uint64, *apperror.Error) {
	return 1, nil
}
func (f *fakeRouterMailService) UpdateTemplate(ctx context.Context, id uint64, input mail.SaveTemplateInput) *apperror.Error {
	return nil
}
func (f *fakeRouterMailService) ChangeTemplateStatus(ctx context.Context, id uint64, status int) *apperror.Error {
	return nil
}
func (f *fakeRouterMailService) DeleteTemplate(ctx context.Context, id uint64) *apperror.Error {
	return nil
}
func (f *fakeRouterMailService) Logs(ctx context.Context, query mail.LogQuery) (*mail.LogListResponse, *apperror.Error) {
	return &mail.LogListResponse{List: []mail.LogDTO{{ID: 1, Scene: enum.VerifyCodeSceneLogin, ToEmail: "user@example.com"}}}, nil
}
func (f *fakeRouterMailService) Log(ctx context.Context, id uint64) (*mail.LogDTO, *apperror.Error) {
	return &mail.LogDTO{ID: id, Scene: enum.VerifyCodeSceneLogin, ToEmail: "user@example.com"}, nil
}
func (f *fakeRouterMailService) DeleteLogs(ctx context.Context, ids []uint64) *apperror.Error {
	f.deletedLogs = ids
	return nil
}

type fakeRouterSmsService struct {
	initCalled  bool
	savedConfig sms.SaveConfigInput
	deletedLogs []uint64
}

func (f *fakeRouterSmsService) PageInit(ctx context.Context) (*sms.PageInitResponse, *apperror.Error) {
	f.initCalled = true
	return &sms.PageInitResponse{Dict: sms.PageInitDict{DefaultRegion: sms.DefaultRegion, DefaultEndpoint: sms.DefaultEndpoint}}, nil
}

func (f *fakeRouterSmsService) Config(ctx context.Context) (*sms.ConfigResponse, *apperror.Error) {
	return &sms.ConfigResponse{Configured: true, Region: sms.DefaultRegion, Endpoint: sms.DefaultEndpoint, SmsSdkAppID: "1400000000", SignName: "签名"}, nil
}

func (f *fakeRouterSmsService) SaveConfig(ctx context.Context, input sms.SaveConfigInput) *apperror.Error {
	f.savedConfig = input
	return nil
}

func (f *fakeRouterSmsService) DeleteConfig(ctx context.Context) *apperror.Error { return nil }
func (f *fakeRouterSmsService) TestSend(ctx context.Context, input sms.TestInput) *apperror.Error {
	return nil
}
func (f *fakeRouterSmsService) Templates(ctx context.Context) ([]sms.TemplateDTO, *apperror.Error) {
	return []sms.TemplateDTO{{ID: 1, Scene: enum.VerifyCodeSceneLogin, Name: "登录验证码"}}, nil
}
func (f *fakeRouterSmsService) CreateTemplate(ctx context.Context, input sms.SaveTemplateInput) (uint64, *apperror.Error) {
	return 1, nil
}
func (f *fakeRouterSmsService) UpdateTemplate(ctx context.Context, id uint64, input sms.SaveTemplateInput) *apperror.Error {
	return nil
}
func (f *fakeRouterSmsService) ChangeTemplateStatus(ctx context.Context, id uint64, status int) *apperror.Error {
	return nil
}
func (f *fakeRouterSmsService) DeleteTemplate(ctx context.Context, id uint64) *apperror.Error {
	return nil
}
func (f *fakeRouterSmsService) Logs(ctx context.Context, query sms.LogQuery) (*sms.LogListResponse, *apperror.Error) {
	return &sms.LogListResponse{List: []sms.LogDTO{{ID: 1, Scene: enum.VerifyCodeSceneLogin, ToPhone: "+8613800138000"}}}, nil
}
func (f *fakeRouterSmsService) Log(ctx context.Context, id uint64) (*sms.LogDTO, *apperror.Error) {
	return &sms.LogDTO{ID: id, Scene: enum.VerifyCodeSceneLogin, ToPhone: "+8613800138000"}, nil
}
func (f *fakeRouterSmsService) DeleteLogs(ctx context.Context, ids []uint64) *apperror.Error {
	f.deletedLogs = ids
	return nil
}

type fakeRouterPaymentService struct {
	configListQuery payment.ConfigListQuery
	orderListQuery  payment.OrderListQuery
	rechargeQuery   payment.RechargeListQuery
	createInput     payment.ConfigMutationInput
	orderInput      payment.OrderCreateInput
	rechargeInput   payment.RechargeCreateInput
	updateID        int64
	statusID        int64
	status          int
	deleteID        int64
	testID          int64
	uploadInput     payment.CertificateUploadInput
	orderID         int64
	rechargeID      int64
	payID           int64
	syncID          int64
	closeID         int64
	callbackCalled  bool
	callbackInput   payment.AlipayCallbackInput
}

func (f *fakeRouterPaymentService) ConfigPageInit(ctx context.Context) (*payment.ConfigPageInitResponse, *apperror.Error) {
	return payment.NewService(payment.Dependencies{}).ConfigPageInit(ctx)
}

func (f *fakeRouterPaymentService) ListConfigs(ctx context.Context, query payment.ConfigListQuery) (*payment.ConfigListResponse, *apperror.Error) {
	f.configListQuery = query
	return &payment.ConfigListResponse{List: []payment.ConfigListItem{{ID: 1, Code: "alipay_default", Name: "支付宝"}}, Page: payment.Page{CurrentPage: query.CurrentPage, PageSize: query.PageSize, Total: 1, TotalPage: 1}}, nil
}

func (f *fakeRouterPaymentService) CreateConfig(ctx context.Context, input payment.ConfigMutationInput) (int64, *apperror.Error) {
	f.createInput = input
	return 11, nil
}

func (f *fakeRouterPaymentService) UpdateConfig(ctx context.Context, id int64, input payment.ConfigMutationInput) *apperror.Error {
	f.updateID = id
	return nil
}

func (f *fakeRouterPaymentService) ChangeConfigStatus(ctx context.Context, id int64, status int) *apperror.Error {
	f.statusID = id
	f.status = status
	return nil
}

func (f *fakeRouterPaymentService) DeleteConfig(ctx context.Context, id int64) *apperror.Error {
	f.deleteID = id
	return nil
}

func (f *fakeRouterPaymentService) UploadCertificate(ctx context.Context, input payment.CertificateUploadInput) (*payment.CertificateUploadResponse, *apperror.Error) {
	f.uploadInput = input
	return &payment.CertificateUploadResponse{Path: "runtime/payment/certs/alipay/alipay_default/abc.crt", FileName: input.FileName, SHA256: "abc", Size: input.Size}, nil
}

func (f *fakeRouterPaymentService) TestConfig(ctx context.Context, id int64) (*payment.ConfigTestResponse, *apperror.Error) {
	f.testID = id
	return &payment.ConfigTestResponse{OK: true, Checks: []string{"local"}, Message: "ok"}, nil
}
func (f *fakeRouterPaymentService) OrderPageInit(ctx context.Context) (*payment.OrderPageInitResponse, *apperror.Error) {
	return &payment.OrderPageInitResponse{Dict: payment.OrderPageInitDict{}, ConfigOptions: []payment.OrderConfigOption{}}, nil
}
func (f *fakeRouterPaymentService) ListOrders(ctx context.Context, query payment.OrderListQuery) (*payment.OrderListResponse, *apperror.Error) {
	f.orderListQuery = query
	return &payment.OrderListResponse{List: []payment.OrderListItem{{ID: 1, OrderNo: "PAY20260515100000000000", ConfigCode: "alipay_default", Status: "paying", PayURL: "https://pay.example.test"}}, Page: payment.Page{CurrentPage: query.CurrentPage, PageSize: query.PageSize, Total: 1, TotalPage: 1}}, nil
}
func (f *fakeRouterPaymentService) GetOrder(ctx context.Context, id int64) (*payment.OrderDetail, *apperror.Error) {
	f.orderID = id
	return &payment.OrderDetail{OrderListItem: payment.OrderListItem{ID: id, OrderNo: "PAY20260515100000000000", ConfigCode: "alipay_default", Status: "pending"}}, nil
}
func (f *fakeRouterPaymentService) CreateOrder(ctx context.Context, input payment.OrderCreateInput) (*payment.OrderCreateResponse, *apperror.Error) {
	f.orderInput = input
	return &payment.OrderCreateResponse{ID: 1, OrderNo: "PAY20260515100000000000", Status: "pending"}, nil
}
func (f *fakeRouterPaymentService) PayOrder(ctx context.Context, id int64) (*payment.OrderPayResponse, *apperror.Error) {
	f.payID = id
	return &payment.OrderPayResponse{ID: id, OrderNo: "PAY20260515100000000000", Status: "paying", PayURL: "https://pay.example.test"}, nil
}
func (f *fakeRouterPaymentService) SyncOrder(ctx context.Context, id int64) (*payment.OrderStatusResponse, *apperror.Error) {
	f.syncID = id
	return &payment.OrderStatusResponse{ID: id, OrderNo: "PAY20260515100000000000", Status: "paying", StatusText: "支付中"}, nil
}
func (f *fakeRouterPaymentService) CloseOrder(ctx context.Context, id int64) (*payment.OrderStatusResponse, *apperror.Error) {
	f.closeID = id
	return &payment.OrderStatusResponse{ID: id, OrderNo: "PAY20260515100000000000", Status: "closed", StatusText: "已关闭"}, nil
}
func (f *fakeRouterPaymentService) RechargePageInit(ctx context.Context, userID int64) (*payment.RechargePageInitResponse, *apperror.Error) {
	return &payment.RechargePageInitResponse{
		Wallet:        payment.WalletSummary{},
		Packages:      []payment.RechargePackageItem{{Code: "recharge_10", Name: "¥10", AmountCents: 1000, AmountText: "10.00"}},
		PaymentMethod: payment.RechargePaymentMethod{Provider: "alipay", Label: "支付宝", Enabled: true},
	}, nil
}
func (f *fakeRouterPaymentService) ListRecharges(ctx context.Context, query payment.RechargeListQuery) (*payment.RechargeListResponse, *apperror.Error) {
	f.rechargeQuery = query
	return &payment.RechargeListResponse{
		List: []payment.RechargeListItem{{ID: 1, RechargeNo: "RCG20260515100000000000", PaymentOrderNo: "PAY20260515100000000000", PackageName: "¥10", AmountCents: 1000, AmountText: "10.00", Status: "paying", PayURL: "https://pay.example.test"}},
		Page: payment.Page{CurrentPage: query.CurrentPage, PageSize: query.PageSize, Total: 1, TotalPage: 1},
	}, nil
}
func (f *fakeRouterPaymentService) GetRecharge(ctx context.Context, userID int64, id int64) (*payment.RechargeDetail, *apperror.Error) {
	f.rechargeID = id
	return &payment.RechargeDetail{RechargeListItem: payment.RechargeListItem{ID: id, RechargeNo: "RCG20260515100000000000", Status: "paying"}}, nil
}
func (f *fakeRouterPaymentService) CreateRecharge(ctx context.Context, input payment.RechargeCreateInput) (*payment.RechargePayResponse, *apperror.Error) {
	f.rechargeInput = input
	return &payment.RechargePayResponse{ID: 1, RechargeNo: "RCG20260515100000000000", PaymentOrderNo: "PAY20260515100000000000", Status: "paying", PayURL: "https://pay.example.test"}, nil
}
func (f *fakeRouterPaymentService) PayRecharge(ctx context.Context, userID int64, id int64) (*payment.RechargePayResponse, *apperror.Error) {
	f.payID = id
	return &payment.RechargePayResponse{ID: id, RechargeNo: "RCG20260515100000000000", PaymentOrderNo: "PAY20260515100000000000", Status: "paying", PayURL: "https://pay.example.test"}, nil
}
func (f *fakeRouterPaymentService) SyncRecharge(ctx context.Context, userID int64, id int64) (*payment.RechargeStatusResponse, *apperror.Error) {
	f.syncID = id
	return &payment.RechargeStatusResponse{ID: id, RechargeNo: "RCG20260515100000000000", Status: "credited", StatusText: "已入账"}, nil
}
func (f *fakeRouterPaymentService) CloseRecharge(ctx context.Context, userID int64, id int64) (*payment.RechargeStatusResponse, *apperror.Error) {
	f.closeID = id
	return &payment.RechargeStatusResponse{ID: id, RechargeNo: "RCG20260515100000000000", Status: "closed", StatusText: "已关闭"}, nil
}
func (f *fakeRouterPaymentService) HandleAlipayCallback(ctx context.Context, input payment.AlipayCallbackInput) (*payment.AlipayCallbackResult, *apperror.Error) {
	f.callbackCalled = true
	f.callbackInput = input
	return &payment.AlipayCallbackResult{Text: "success"}, nil
}

type fakeRouterUploadTokenService struct {
	input uploadtoken.CreateInput
}

func (f *fakeRouterUploadTokenService) Create(ctx context.Context, input uploadtoken.CreateInput) (*uploadtoken.CreateResponse, *apperror.Error) {
	f.input = input
	return &uploadtoken.CreateResponse{
		Provider: "cos",
		Bucket:   "bucket-a",
		Region:   "ap-nanjing",
		Key:      "images/2026/05/05/demo.png",
		Credentials: uploadtoken.CredentialsDTO{
			TmpSecretID:  "tmp-id",
			TmpSecretKey: "tmp-key",
			SessionToken: "session-token",
		},
		StartTime:   100,
		ExpiredTime: 200,
		Rule: uploadtoken.UploadRuleDTO{
			MaxSizeMB: 2,
			ImageExts: []string{
				"png",
			},
			FileExts: []string{
				"pdf",
			},
		},
	}, nil
}

type fakeRouterCanvasService struct {
	settings canvasmodule.SettingsInput
}

func (f *fakeRouterCanvasService) PublicSettings(ctx context.Context, input canvasmodule.SettingsInput) (*canvasmodule.SettingsResponse, *apperror.Error) {
	f.settings = input
	return &canvasmodule.SettingsResponse{AllowRegister: true, Scenes: []string{"canvas_text_generate"}}, nil
}

type fakeRouterAiPromptService struct {
	query aiprompt.ListQuery
}

func (f *fakeRouterAiPromptService) PublicList(ctx context.Context, query aiprompt.ListQuery) (*aiprompt.ListResponse, *apperror.Error) {
	f.query = query
	return &aiprompt.ListResponse{List: []aiprompt.Item{{ID: 1, Slug: "prompt", Title: "Prompt"}}}, nil
}

type fakeRouterAiAssetService struct {
	userID      uint64
	query       aiasset.ListQuery
	created     aiasset.Input
	updatedID   int64
	updated     aiasset.Input
	deletedID   int64
	createCalls int
	updateCalls int
	deleteCalls int
}

func (f *fakeRouterAiAssetService) UserList(ctx context.Context, userID uint64, query aiasset.ListQuery) (*aiasset.ListResponse, *apperror.Error) {
	f.userID = userID
	f.query = query
	return &aiasset.ListResponse{List: []aiasset.Item{{ID: 2, Slug: "asset", Type: aiasset.AssetTypeImage, Title: "Asset"}}}, nil
}
func (f *fakeRouterAiAssetService) UserCreate(ctx context.Context, userID uint64, input aiasset.Input) (int64, *apperror.Error) {
	f.userID = userID
	f.created = input
	f.createCalls++
	return 22, nil
}
func (f *fakeRouterAiAssetService) UserUpdate(ctx context.Context, userID uint64, id int64, input aiasset.Input) *apperror.Error {
	f.userID = userID
	f.updatedID = id
	f.updated = input
	f.updateCalls++
	return nil
}
func (f *fakeRouterAiAssetService) UserDelete(ctx context.Context, userID uint64, id int64) *apperror.Error {
	f.userID = userID
	f.deletedID = id
	f.deleteCalls++
	return nil
}

type fakeRouterAdminAIPromptService struct {
	listQuery       aiprompt.ListQuery
	detailID        int64
	created         aiprompt.Input
	updatedID       int64
	updated         aiprompt.Input
	statusID        int64
	status          int
	deletedID       int64
	batchDeletedIDs []int64
}

func (f *fakeRouterAdminAIPromptService) PageInit(ctx context.Context) (*aiprompt.PageInitResponse, *apperror.Error) {
	return &aiprompt.PageInitResponse{}, nil
}
func (f *fakeRouterAdminAIPromptService) List(ctx context.Context, query aiprompt.ListQuery) (*aiprompt.ListResponse, *apperror.Error) {
	f.listQuery = query
	return &aiprompt.ListResponse{List: []aiprompt.Item{{ID: 1, Slug: "prompt", Title: "Prompt", Prompt: "text"}}}, nil
}
func (f *fakeRouterAdminAIPromptService) Detail(ctx context.Context, id int64) (*aiprompt.Item, *apperror.Error) {
	f.detailID = id
	return &aiprompt.Item{ID: id, Slug: "prompt", Title: "Prompt", Prompt: "text"}, nil
}
func (f *fakeRouterAdminAIPromptService) Create(ctx context.Context, input aiprompt.Input) (int64, *apperror.Error) {
	f.created = input
	return 31, nil
}
func (f *fakeRouterAdminAIPromptService) Update(ctx context.Context, id int64, input aiprompt.Input) *apperror.Error {
	f.updatedID = id
	f.updated = input
	return nil
}
func (f *fakeRouterAdminAIPromptService) ChangeStatus(ctx context.Context, id int64, status int) *apperror.Error {
	f.statusID = id
	f.status = status
	return nil
}
func (f *fakeRouterAdminAIPromptService) DeleteOne(ctx context.Context, id int64) *apperror.Error {
	f.deletedID = id
	return nil
}
func (f *fakeRouterAdminAIPromptService) DeleteBatch(ctx context.Context, ids []int64) *apperror.Error {
	f.batchDeletedIDs = append([]int64(nil), ids...)
	return nil
}

type fakeRouterWalletService struct {
	summaryUserID int64
	query         walletmodule.TransactionListQuery
}

func (f *fakeRouterWalletService) Summary(ctx context.Context, userID int64) (*walletmodule.SummaryResponse, *apperror.Error) {
	f.summaryUserID = userID
	return &walletmodule.SummaryResponse{BalanceCents: 1200, BalanceText: "12.00"}, nil
}
func (f *fakeRouterWalletService) Transactions(ctx context.Context, query walletmodule.TransactionListQuery) (*walletmodule.TransactionListResponse, *apperror.Error) {
	f.query = query
	return &walletmodule.TransactionListResponse{List: []walletmodule.TransactionItem{{ID: 1, UserID: query.UserID, TransactionNo: "WLT1", AmountCents: 100}}, Page: walletmodule.Page{CurrentPage: query.CurrentPage, PageSize: query.PageSize, Total: 1, TotalPage: 1}}, nil
}
func (f *fakeRouterWalletService) WalletUsersPageInit(ctx context.Context) (*walletmodule.WalletUsersPageInitResponse, *apperror.Error) {
	return &walletmodule.WalletUsersPageInitResponse{}, nil
}
func (f *fakeRouterWalletService) WalletUsers(ctx context.Context, query walletmodule.WalletUserListQuery) (*walletmodule.WalletUserListResponse, *apperror.Error) {
	return &walletmodule.WalletUserListResponse{}, nil
}
func (f *fakeRouterWalletService) LedgerPageInit(ctx context.Context) (*walletmodule.LedgerPageInitResponse, *apperror.Error) {
	return &walletmodule.LedgerPageInitResponse{}, nil
}
func (f *fakeRouterWalletService) Ledger(ctx context.Context, query walletmodule.TransactionListQuery) (*walletmodule.TransactionListResponse, *apperror.Error) {
	return &walletmodule.TransactionListResponse{}, nil
}

type fakeRouterQueueMonitorService struct {
	listCalled      bool
	failedListQuery queuemonitor.FailedListQuery
}

type fakeQueueMonitorUI struct {
	called bool
	path   string
	method string
}

func (f *fakeQueueMonitorUI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.called = true
	f.path = r.URL.Path
	f.method = r.Method
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("queue monitor ui"))
}

func (f *fakeRouterQueueMonitorService) List(ctx context.Context) ([]queuemonitor.QueueItem, *apperror.Error) {
	f.listCalled = true
	return []queuemonitor.QueueItem{{Name: "critical", Label: "高优先级队列", Group: "critical"}}, nil
}

func (f *fakeRouterQueueMonitorService) FailedList(ctx context.Context, query queuemonitor.FailedListQuery) (*queuemonitor.FailedListResponse, *apperror.Error) {
	f.failedListQuery = query
	return &queuemonitor.FailedListResponse{
		List: []queuemonitor.FailedTaskItem{{ID: "task-1", State: "retry"}},
		Page: queuemonitor.Page{CurrentPage: query.CurrentPage, PageSize: query.PageSize, Total: 1, TotalPage: 1},
	}, nil
}

func TestHealthEndpointReturnsOK(t *testing.T) {
	router := newTestRouter(t, testDependencies{})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	router.ServeHTTP(recorder, request)

	assertRequestID(t, recorder)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}

	body := decodeRouterBody(t, recorder)
	if body["code"] != float64(0) {
		t.Fatalf("expected code 0, got %#v", body["code"])
	}
	if body["msg"] != "ok" {
		t.Fatalf("expected msg ok, got %#v", body["msg"])
	}

	data := mustRouterData(t, body)
	if data["status"] != "ok" {
		t.Fatalf("expected data.status ok, got %#v", data["status"])
	}
}

func TestPingEndpointReturnsPong(t *testing.T) {
	router := newTestRouter(t, testDependencies{})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/ping", nil)
	router.ServeHTTP(recorder, request)

	assertRequestID(t, recorder)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}

	body := decodeRouterBody(t, recorder)
	data := mustRouterData(t, body)
	if data["message"] != "pong" {
		t.Fatalf("expected data.message pong, got %#v", data["message"])
	}
}

func TestReadyEndpointReturnsReadyWhenResourcesAreDisabled(t *testing.T) {
	router := newTestRouter(t, testDependencies{})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/ready", nil)
	router.ServeHTTP(recorder, request)

	assertRequestID(t, recorder)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}

	body := decodeRouterBody(t, recorder)
	if body["code"] != float64(0) {
		t.Fatalf("expected code 0, got %#v", body["code"])
	}

	data := mustRouterData(t, body)
	if data["status"] != readiness.StatusReady {
		t.Fatalf("expected ready status, got %#v", data["status"])
	}
	checks, ok := data["checks"].(map[string]any)
	if !ok {
		t.Fatalf("expected checks object, got %#v", data["checks"])
	}
	database, ok := checks["database"].(map[string]any)
	if !ok || database["status"] != readiness.StatusDisabled {
		t.Fatalf("expected disabled database check, got %#v", checks["database"])
	}
	queueRedis, ok := checks["queue_redis"].(map[string]any)
	if !ok || queueRedis["status"] != readiness.StatusDisabled {
		t.Fatalf("expected disabled queue_redis check, got %#v", checks["queue_redis"])
	}
	realtimeCheck, ok := checks["realtime"].(map[string]any)
	if !ok || realtimeCheck["status"] != readiness.StatusDisabled {
		t.Fatalf("expected disabled realtime check, got %#v", checks["realtime"])
	}
}

func TestReadyEndpointReturnsErrorWithDetailsWhenResourceIsDown(t *testing.T) {
	router := newTestRouter(t, testDependencies{Readiness: fakeReadinessChecker{report: readiness.NewReport(map[string]readiness.Check{
		"database": {Status: readiness.StatusDown, Message: "connection refused"},
		"redis":    {Status: readiness.StatusDisabled},
	})}})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/ready", nil)
	router.ServeHTTP(recorder, request)

	assertRequestID(t, recorder)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("expected status %d, got %d", http.StatusInternalServerError, recorder.Code)
	}

	body := decodeRouterBody(t, recorder)
	if body["code"] != float64(500) {
		t.Fatalf("expected code 500, got %#v", body["code"])
	}
	if body["msg"] != "服务未就绪" {
		t.Fatalf("expected service not ready message, got %#v", body["msg"])
	}

	data := mustRouterData(t, body)
	if data["status"] != readiness.StatusNotReady {
		t.Fatalf("expected not_ready status, got %#v", data["status"])
	}
}

func TestRouterReportsReadyFailureOnce(t *testing.T) {
	var buffer bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buffer, nil))
	router := newRouterFromTestDependencies(testDependencies{
		Logger: logger,
		Readiness: fakeReadinessChecker{report: readiness.NewReport(map[string]readiness.Check{
			"database": {Status: readiness.StatusDown, Message: "connection refused"},
		})},
	})

	request := httptest.NewRequest(http.MethodGet, "/ready", nil)
	request.Header.Set(middleware.HeaderRequestID, "rid-ready")
	request.Header.Set("X-Trace-Id", "trace-ready")
	router.ServeHTTP(httptest.NewRecorder(), request)

	reported := 0
	for _, line := range strings.Split(strings.TrimSpace(buffer.String()), "\n") {
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("invalid router log: %v\n%s", err, line)
		}
		if entry["msg"] != "application request failed" {
			continue
		}
		reported++
		if entry["request_id"] != "rid-ready" || entry["trace_id"] != "trace-ready" {
			t.Fatalf("missing failure correlation: %#v", entry)
		}
		if entry["error_code"] != "internal.unknown" || entry["category"] != "internal" {
			t.Fatalf("missing failure classification: %#v", entry)
		}
	}
	if reported != 1 {
		t.Fatalf("expected one reported server failure, got %d logs=%s", reported, buffer.String())
	}
}

func TestRouterInstallsAccessLogAfterRequestID(t *testing.T) {
	var buffer bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buffer, nil))
	router := newRouterFromTestDependencies(testDependencies{Logger: logger})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	request.Header.Set(middleware.HeaderRequestID, "rid-router")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}

	entry := decodeRouterLogEntry(t, buffer.Bytes())
	if entry["msg"] != "http request" {
		t.Fatalf("expected http request log message, got %#v", entry["msg"])
	}
	if entry["request_id"] != "rid-router" {
		t.Fatalf("expected request_id rid-router, got %#v", entry["request_id"])
	}
	if entry["method"] != http.MethodGet {
		t.Fatalf("expected method GET, got %#v", entry["method"])
	}
	if entry["path"] != "/health" {
		t.Fatalf("expected path /health, got %#v", entry["path"])
	}
	if entry["status"] != float64(http.StatusOK) {
		t.Fatalf("expected status 200, got %#v", entry["status"])
	}
}

func TestRouterInstallsCORSAfterAccessLog(t *testing.T) {
	var buffer bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buffer, nil))
	router := newRouterFromTestDependencies(testDependencies{
		Logger: logger,
		CORS: config.CORSConfig{
			AllowOrigins:  []string{"http://localhost:5173"},
			AllowMethods:  []string{http.MethodGet, http.MethodOptions},
			AllowHeaders:  []string{"Content-Type", "Authorization", "platform", "device-id", "X-Trace-Id", middleware.HeaderRequestID},
			ExposeHeaders: []string{middleware.HeaderRequestID},
			MaxAge:        12 * time.Hour,
		},
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodOptions, "/health", nil)
	request.Header.Set("Origin", "http://localhost:5173")
	request.Header.Set("Access-Control-Request-Method", http.MethodGet)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, recorder.Code)
	}
	if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
		t.Fatalf("expected allowed origin, got %q", got)
	}

	entry := decodeRouterLogEntry(t, buffer.Bytes())
	if entry["msg"] != "http request" {
		t.Fatalf("expected http request log message, got %#v", entry["msg"])
	}
	if entry["status"] != float64(http.StatusNoContent) {
		t.Fatalf("expected access log status 204, got %#v", entry["status"])
	}
}

func TestRouterInstallsAuthTokenForNonPublicPaths(t *testing.T) {
	router := newTestRouter(t, testDependencies{})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/private", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusUnauthorized, recorder.Code, recorder.Body.String())
	}
	body := decodeRouterBody(t, recorder)
	if body["code"] != float64(401) {
		t.Fatalf("expected code 401, got %#v", body["code"])
	}
	if body["msg"] != "缺少Token" {
		t.Fatalf("expected missing token message, got %#v", body["msg"])
	}
}

func TestRouterInstallsRefreshEndpointAsPublicPath(t *testing.T) {
	router := newTestRouter(t, testDependencies{AuthService: fakeAuthService{}})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/v1/auth/refresh", strings.NewReader(`{"refresh_token":"refresh-token"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(auth.ClientVariantHeader, string(auth.ClientDesktop))
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	body := decodeRouterBody(t, recorder)
	data := mustRouterData(t, body)
	if data["access_token"] != "new-access" {
		t.Fatalf("expected refresh endpoint response, got %#v", data)
	}
}

func TestRouterRefreshEndpointIncludesCORSHeaders(t *testing.T) {
	router := newTestRouter(t, testDependencies{
		CORS:        config.DefaultCORSConfig(),
		AuthService: fakeAuthService{},
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/v1/auth/refresh", strings.NewReader(`{"refresh_token":"refresh-token"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "http://127.0.0.1:5173")
	request.Header.Set(auth.ClientVariantHeader, string(auth.ClientDesktop))
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != "http://127.0.0.1:5173" {
		t.Fatalf("expected refresh CORS allow origin, got %q", got)
	}
	if got := recorder.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Fatalf("expected refresh CORS credentials true, got %q", got)
	}
}

func TestRouterInstallsLoginEndpointsAsPublicPaths(t *testing.T) {
	router := newTestRouter(t, testDependencies{AuthService: fakeAuthService{}})

	configRecorder := httptest.NewRecorder()
	configRequest := httptest.NewRequest(http.MethodGet, "/api/admin/v1/auth/login-config", nil)
	configRequest.Header.Set("platform", "admin")
	router.ServeHTTP(configRecorder, configRequest)
	if configRecorder.Code != http.StatusOK {
		t.Fatalf("expected login config status %d, got %d body=%s", http.StatusOK, configRecorder.Code, configRecorder.Body.String())
	}

	loginRecorder := httptest.NewRecorder()
	loginRequest := httptest.NewRequest(http.MethodPost, "/api/admin/v1/auth/login", strings.NewReader(`{"login_account":"15671628271","login_type":"password","password":"123456","captcha_id":"captcha-id","captcha_answer":{"x":120,"y":80}}`))
	loginRequest.Header.Set("Content-Type", "application/json")
	loginRequest.Header.Set("platform", "admin")
	loginRequest.Header.Set(auth.ClientVariantHeader, string(auth.ClientDesktop))
	router.ServeHTTP(loginRecorder, loginRequest)
	if loginRecorder.Code != http.StatusOK {
		t.Fatalf("expected login status %d, got %d body=%s", http.StatusOK, loginRecorder.Code, loginRecorder.Body.String())
	}

	forgotRecorder := httptest.NewRecorder()
	forgotRequest := httptest.NewRequest(http.MethodPost, "/api/admin/v1/auth/forgot-password", strings.NewReader(`{"account":"15671628271","code":"123456","new_password":"new-secret","confirm_password":"new-secret"}`))
	forgotRequest.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(forgotRecorder, forgotRequest)
	if forgotRecorder.Code != http.StatusOK {
		t.Fatalf("expected forgot password status %d, got %d body=%s", http.StatusOK, forgotRecorder.Code, forgotRecorder.Body.String())
	}
}

func TestRouterInstallsCaptchaEndpointAsPublicPath(t *testing.T) {
	router := newTestRouter(t, testDependencies{CaptchaService: fakeCaptchaService{}})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/auth/captcha", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected captcha status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	body := decodeRouterBody(t, recorder)
	data := mustRouterData(t, body)
	if data["captcha_id"] != "captcha-id" || data["captcha_type"] != auth.TypeSlide {
		t.Fatalf("unexpected captcha response: %#v", data)
	}
}

func TestRouterInstallsUsersMeAsProtectedPath(t *testing.T) {
	var authInput middleware.TokenInput
	userService := &fakeRouterUserService{result: &user.InitResponse{
		UserID:      1,
		Username:    "admin",
		Avatar:      "avatar.png",
		RoleName:    "管理员",
		Permissions: []permission.MenuItem{{Index: "1", Label: "系统", Children: []permission.MenuItem{}}},
		Router:      []permission.RouteItem{{Name: "menu_2", Path: "/system/user", ViewKey: "system/user/index"}},
		ButtonCodes: []string{"user_add"},
	}}
	router := newTestRouter(t, testDependencies{
		Authenticator: func(ctx context.Context, input middleware.TokenInput) (*middleware.AuthIdentity, *apperror.Error) {
			authInput = input
			return &middleware.AuthIdentity{UserID: 1, SessionID: 10, Platform: input.Platform}, nil
		},
		UserService: userService,
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/users/me", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	request.Header.Set("platform", "admin")
	request.Header.Set("device-id", "desktop-1")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if authInput.AccessToken != "access-token" || authInput.Platform != "admin" || authInput.DeviceID != "desktop-1" {
		t.Fatalf("unexpected auth input: %#v", authInput)
	}
	if userService.input.UserID != 1 || userService.input.Platform != "admin" {
		t.Fatalf("unexpected user service input: %#v", userService.input)
	}
	body := decodeRouterBody(t, recorder)
	data := mustRouterData(t, body)
	if data["username"] != "admin" || data["role_name"] != "管理员" {
		t.Fatalf("unexpected users/me payload: %#v", data)
	}
	if _, ok := data["buttonCodes"]; !ok {
		t.Fatalf("missing buttonCodes in users/me payload: %#v", data)
	}
}

func TestRouterInstallsAppAuthRoutes(t *testing.T) {
	var authInput auth.LoginInput
	var loginConfigPlatform string
	var sendCodeInput auth.SendCodeInput
	var tokenInput middleware.TokenInput
	var logoutToken string
	userService := &fakeRouterUserService{result: &user.InitResponse{
		UserID:      7,
		Username:    "移动端用户",
		Avatar:      "avatar.png",
		RoleName:    "移动端角色",
		Permissions: []permission.MenuItem{{Index: "app_home", Label: "App Home", Path: "/app/home"}},
		Router:      []permission.RouteItem{{Name: "AppHome", Path: "/app/home", ViewKey: "app/home"}},
		ButtonCodes: []string{"app_access"},
	}}
	authService := &fakeAppRouterAuthService{
		loginConfigFn: func(ctx context.Context, platform string) (*auth.LoginConfigResponse, *apperror.Error) {
			loginConfigPlatform = platform
			return &auth.LoginConfigResponse{
				LoginTypeArr:   []auth.LoginTypeOption{{Label: "密码登录", Value: auth.LoginTypePassword}},
				CaptchaEnabled: true,
				CaptchaType:    auth.TypeSlide,
			}, nil
		},
		sendCodeFn: func(ctx context.Context, input auth.SendCodeInput) (string, *apperror.Error) {
			sendCodeInput = input
			return "验证码发送成功", nil
		},
		loginFn: func(ctx context.Context, input auth.LoginInput) (*auth.LoginResponse, *apperror.Error) {
			authInput = input
			return &auth.LoginResponse{UserID: 7, AccessToken: "app-token", ExpiresIn: 14400}, nil
		},
		logoutFn: func(ctx context.Context, accessToken string) *apperror.Error {
			logoutToken = accessToken
			return nil
		},
	}
	router := newTestRouter(t, testDependencies{
		AuthService:    authService,
		CaptchaService: fakeCaptchaService{},
		Authenticator: func(ctx context.Context, input middleware.TokenInput) (*middleware.AuthIdentity, *apperror.Error) {
			tokenInput = input
			return &middleware.AuthIdentity{UserID: 7, SessionID: 20, Platform: input.Platform}, nil
		},
		UserService: userService,
	})

	configRecorder := httptest.NewRecorder()
	configRequest := httptest.NewRequest(http.MethodGet, "/api/app/v1/auth/login-config", nil)
	configRequest.Header.Set("platform", "admin")
	router.ServeHTTP(configRecorder, configRequest)
	if configRecorder.Code != http.StatusOK {
		t.Fatalf("expected app login-config status %d, got %d body=%s", http.StatusOK, configRecorder.Code, configRecorder.Body.String())
	}
	if loginConfigPlatform != enum.PlatformApp {
		t.Fatalf("expected app login-config to force platform app, got %q", loginConfigPlatform)
	}
	configData := mustRouterData(t, decodeRouterBody(t, configRecorder))
	if configData["captcha_type"] != auth.TypeSlide || configData["captcha_enabled"] != true {
		t.Fatalf("unexpected app login-config payload: %#v", configData)
	}

	captchaRecorder := httptest.NewRecorder()
	captchaRequest := httptest.NewRequest(http.MethodGet, "/api/app/v1/auth/captcha", nil)
	router.ServeHTTP(captchaRecorder, captchaRequest)
	if captchaRecorder.Code != http.StatusOK {
		t.Fatalf("expected app captcha status %d, got %d body=%s", http.StatusOK, captchaRecorder.Code, captchaRecorder.Body.String())
	}
	captchaData := mustRouterData(t, decodeRouterBody(t, captchaRecorder))
	if captchaData["captcha_id"] != "captcha-id" || captchaData["captcha_type"] != auth.TypeSlide {
		t.Fatalf("unexpected app captcha payload: %#v", captchaData)
	}

	sendCodeRecorder := httptest.NewRecorder()
	sendCodeRequest := httptest.NewRequest(http.MethodPost, "/api/app/v1/auth/send-code", strings.NewReader(`{"account":"15671628271","scene":"login"}`))
	sendCodeRequest.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(sendCodeRecorder, sendCodeRequest)
	if sendCodeRecorder.Code != http.StatusOK {
		t.Fatalf("expected app send-code status %d, got %d body=%s", http.StatusOK, sendCodeRecorder.Code, sendCodeRecorder.Body.String())
	}
	if sendCodeInput.Account != "15671628271" || sendCodeInput.Scene != auth.VerifyCodeSceneLogin {
		t.Fatalf("unexpected app send-code input: %#v", sendCodeInput)
	}

	loginRecorder := httptest.NewRecorder()
	loginRequest := httptest.NewRequest(http.MethodPost, "/api/app/v1/auth/login", strings.NewReader(`{"login_type":"password","login_account":"15671628271","password":"123456","captcha_id":"captcha-id","captcha_answer":{"x":120,"y":80}}`))
	loginRequest.Header.Set("Content-Type", "application/json")
	loginRequest.Header.Set("device-id", "ios-1")
	router.ServeHTTP(loginRecorder, loginRequest)

	if loginRecorder.Code != http.StatusOK {
		t.Fatalf("expected app login status %d, got %d body=%s", http.StatusOK, loginRecorder.Code, loginRecorder.Body.String())
	}
	if authInput.LoginAccount != "15671628271" || authInput.LoginType != auth.LoginTypePassword || authInput.Password != "123456" || authInput.Platform != enum.PlatformApp || authInput.DeviceID != "ios-1" {
		t.Fatalf("unexpected app login input: %#v", authInput)
	}
	if authInput.CaptchaID != "captcha-id" || authInput.CaptchaAnswer == nil || authInput.CaptchaAnswer.X != 120 || authInput.CaptchaAnswer.Y != 80 {
		t.Fatalf("unexpected app login input: %#v", authInput)
	}
	loginBody := decodeRouterBody(t, loginRecorder)
	loginData := mustRouterData(t, loginBody)
	if loginData["token"] != "app-token" {
		t.Fatalf("expected app token response, got %#v", loginData)
	}
	loginUser, ok := loginData["user"].(map[string]any)
	if !ok || loginUser["user_id"] != float64(7) || loginUser["username"] != "移动端用户" || loginUser["avatar"] != "avatar.png" || loginUser["role_name"] != "移动端角色" {
		t.Fatalf("unexpected app login user payload: %#v", loginData["user"])
	}
	if _, ok := loginUser["permissions"].([]any); !ok {
		t.Fatalf("expected permissions in app login user payload: %#v", loginUser)
	}
	if !routerRouteSliceContains(loginUser["router"], "/app/home") {
		t.Fatalf("expected router in app login user payload: %#v", loginUser)
	}
	if !routerStringSliceEqual(loginUser["buttonCodes"], []string{"app_access"}) {
		t.Fatalf("expected buttonCodes in app login user payload: %#v", loginUser)
	}
	for _, forbidden := range []string{"id", "nickname", "quick_entry", "quickEntry", "permissionCodes", "permission_codes", "button_codes"} {
		if _, ok := loginUser[forbidden]; ok {
			t.Fatalf("app login user response must not include alias/admin-only field %q: %#v", forbidden, loginUser)
		}
	}
	if _, ok := loginData["access_token"]; ok {
		t.Fatalf("app login response must not leak admin token field names: %#v", loginData)
	}

	meRecorder := httptest.NewRecorder()
	meRequest := httptest.NewRequest(http.MethodGet, "/api/app/v1/users/me", nil)
	meRequest.Header.Set("Authorization", "Bearer app-token")
	meRequest.Header.Set("device-id", "ios-1")
	router.ServeHTTP(meRecorder, meRequest)
	if meRecorder.Code != http.StatusOK {
		t.Fatalf("expected app users/me status %d, got %d body=%s", http.StatusOK, meRecorder.Code, meRecorder.Body.String())
	}
	if tokenInput.AccessToken != "app-token" || tokenInput.Platform != enum.PlatformApp || tokenInput.DeviceID != "ios-1" {
		t.Fatalf("unexpected app token input: %#v", tokenInput)
	}
	if userService.input.UserID != 7 || userService.input.Platform != enum.PlatformApp {
		t.Fatalf("unexpected app user service input: %#v", userService.input)
	}
	meBody := decodeRouterBody(t, meRecorder)
	meData := mustRouterData(t, meBody)
	if meData["user_id"] != float64(7) || meData["username"] != "移动端用户" || meData["avatar"] != "avatar.png" || meData["role_name"] != "移动端角色" {
		t.Fatalf("unexpected app users/me payload: %#v", meData)
	}
	if _, ok := meData["permissions"].([]any); !ok {
		t.Fatalf("expected permissions array in app users/me payload: %#v", meData)
	}
	if !routerRouteSliceContains(meData["router"], "/app/home") {
		t.Fatalf("expected router in app users/me payload: %#v", meData)
	}
	if !routerStringSliceEqual(meData["buttonCodes"], []string{"app_access"}) {
		t.Fatalf("expected buttonCodes in app users/me payload: %#v", meData)
	}
	for _, forbidden := range []string{"id", "nickname", "quick_entry", "quickEntry", "permissionCodes", "permission_codes", "button_codes"} {
		if _, ok := meData[forbidden]; ok {
			t.Fatalf("app users/me response must not include alias/admin-only field %q: %#v", forbidden, meData)
		}
	}

	logoutRecorder := httptest.NewRecorder()
	logoutRequest := httptest.NewRequest(http.MethodPost, "/api/app/v1/auth/logout", nil)
	logoutRequest.Header.Set("Authorization", "Bearer app-token")
	router.ServeHTTP(logoutRecorder, logoutRequest)
	if logoutRecorder.Code != http.StatusOK {
		t.Fatalf("expected app logout status %d, got %d body=%s", http.StatusOK, logoutRecorder.Code, logoutRecorder.Body.String())
	}
	if logoutToken != "app-token" {
		t.Fatalf("expected app logout token app-token, got %q", logoutToken)
	}
	logoutBody := decodeRouterBody(t, logoutRecorder)
	if logoutBody["code"] != float64(0) || logoutBody["msg"] != "ok" {
		t.Fatalf("unexpected app logout response: %#v", logoutBody)
	}
	if _, ok := logoutBody["data"]; !ok || logoutBody["data"] != nil {
		t.Fatalf("expected app logout data null, got %#v", logoutBody["data"])
	}
}

func TestRouterInstallsCanvasAuthAndCurrentUserRoutes(t *testing.T) {
	var authInput auth.LoginInput
	var loginConfigPlatform string
	var tokenInput middleware.TokenInput
	var logoutToken string
	userService := &fakeRouterUserService{result: &user.InitResponse{
		UserID:      8,
		Username:    "画布用户",
		Avatar:      "canvas.png",
		RoleName:    "画布角色",
		Permissions: []permission.MenuItem{{Index: "canvas_home", Label: "Canvas Home", Path: "/canvas/home"}},
		Router:      []permission.RouteItem{{Name: "CanvasHome", Path: "/canvas/home", ViewKey: "canvas/home"}},
		ButtonCodes: []string{"canvas_access"},
	}}
	authService := &fakeAppRouterAuthService{
		loginConfigFn: func(ctx context.Context, platform string) (*auth.LoginConfigResponse, *apperror.Error) {
			loginConfigPlatform = platform
			return &auth.LoginConfigResponse{LoginTypeArr: []auth.LoginTypeOption{{Label: "密码登录", Value: auth.LoginTypePassword}}, CaptchaEnabled: true, CaptchaType: auth.TypeSlide}, nil
		},
		loginFn: func(ctx context.Context, input auth.LoginInput) (*auth.LoginResponse, *apperror.Error) {
			authInput = input
			return &auth.LoginResponse{UserID: 8, AccessToken: "canvas-token", ExpiresIn: 14400}, nil
		},
		logoutFn: func(ctx context.Context, accessToken string) *apperror.Error {
			logoutToken = accessToken
			return nil
		},
	}
	router := newTestRouter(t, testDependencies{
		AuthService:    authService,
		CaptchaService: fakeCaptchaService{},
		Authenticator: func(ctx context.Context, input middleware.TokenInput) (*middleware.AuthIdentity, *apperror.Error) {
			tokenInput = input
			return &middleware.AuthIdentity{UserID: 8, SessionID: 21, Platform: input.Platform}, nil
		},
		UserService: userService,
	})

	configRecorder := httptest.NewRecorder()
	configRequest := httptest.NewRequest(http.MethodGet, "/api/canvas/v1/auth/login-config", nil)
	configRequest.Header.Set("platform", "admin")
	router.ServeHTTP(configRecorder, configRequest)
	if configRecorder.Code != http.StatusOK {
		t.Fatalf("expected canvas login-config status %d, got %d body=%s", http.StatusOK, configRecorder.Code, configRecorder.Body.String())
	}
	if loginConfigPlatform != enum.PlatformCanvas {
		t.Fatalf("expected canvas login-config to force platform canvas, got %q", loginConfigPlatform)
	}

	loginRecorder := httptest.NewRecorder()
	loginRequest := httptest.NewRequest(http.MethodPost, "/api/canvas/v1/auth/login", strings.NewReader(`{"login_type":"password","login_account":"15671628271","password":"123456","captcha_id":"captcha-id","captcha_answer":{"x":120,"y":80}}`))
	loginRequest.Header.Set("Content-Type", "application/json")
	loginRequest.Header.Set("device-id", "web-1")
	router.ServeHTTP(loginRecorder, loginRequest)
	if loginRecorder.Code != http.StatusOK {
		t.Fatalf("expected canvas login status %d, got %d body=%s", http.StatusOK, loginRecorder.Code, loginRecorder.Body.String())
	}
	if authInput.Platform != enum.PlatformCanvas || authInput.DeviceID != "web-1" {
		t.Fatalf("unexpected canvas login input: %#v", authInput)
	}
	loginData := mustRouterData(t, decodeRouterBody(t, loginRecorder))
	if loginData["token"] != "canvas-token" {
		t.Fatalf("expected canvas token response, got %#v", loginData)
	}
	loginUser, ok := loginData["user"].(map[string]any)
	if !ok || loginUser["user_id"] != float64(8) || loginUser["username"] != "画布用户" || loginUser["avatar"] != "canvas.png" || loginUser["role_name"] != "画布角色" {
		t.Fatalf("unexpected canvas login user payload: %#v", loginData["user"])
	}
	if _, ok := loginUser["permissions"].([]any); !ok {
		t.Fatalf("expected permissions in canvas login user payload: %#v", loginUser)
	}
	if !routerRouteSliceContains(loginUser["router"], "/canvas/home") {
		t.Fatalf("expected router in canvas login user payload: %#v", loginUser)
	}
	if !routerStringSliceEqual(loginUser["buttonCodes"], []string{"canvas_access"}) {
		t.Fatalf("expected buttonCodes in canvas login user payload: %#v", loginUser)
	}
	for _, forbidden := range []string{"id", "nickname", "quick_entry", "quickEntry", "permissionCodes", "permission_codes", "button_codes"} {
		if _, ok := loginUser[forbidden]; ok {
			t.Fatalf("canvas login user response must not include alias/admin-only field %q: %#v", forbidden, loginUser)
		}
	}
	if _, ok := loginData["access_token"]; ok {
		t.Fatalf("canvas login response must not leak admin token field names: %#v", loginData)
	}

	meRecorder := httptest.NewRecorder()
	meRequest := httptest.NewRequest(http.MethodGet, "/api/canvas/v1/users/me", nil)
	meRequest.Header.Set("Authorization", "Bearer canvas-token")
	meRequest.Header.Set("device-id", "web-1")
	router.ServeHTTP(meRecorder, meRequest)
	if meRecorder.Code != http.StatusOK {
		t.Fatalf("expected canvas users/me status %d, got %d body=%s", http.StatusOK, meRecorder.Code, meRecorder.Body.String())
	}
	if tokenInput.AccessToken != "canvas-token" || tokenInput.Platform != enum.PlatformCanvas || tokenInput.DeviceID != "web-1" {
		t.Fatalf("unexpected canvas token input: %#v", tokenInput)
	}
	if userService.input.UserID != 8 || userService.input.Platform != enum.PlatformCanvas {
		t.Fatalf("unexpected canvas user service input: %#v", userService.input)
	}
	meBody := decodeRouterBody(t, meRecorder)
	meData := mustRouterData(t, meBody)
	if meData["user_id"] != float64(8) || meData["username"] != "画布用户" || meData["avatar"] != "canvas.png" || meData["role_name"] != "画布角色" {
		t.Fatalf("unexpected canvas users/me payload: %#v", meData)
	}
	if _, ok := meData["permissions"].([]any); !ok {
		t.Fatalf("expected permissions array in canvas users/me payload: %#v", meData)
	}
	if !routerRouteSliceContains(meData["router"], "/canvas/home") {
		t.Fatalf("expected router in canvas users/me payload: %#v", meData)
	}
	if !routerStringSliceEqual(meData["buttonCodes"], []string{"canvas_access"}) {
		t.Fatalf("expected buttonCodes in canvas users/me payload: %#v", meData)
	}
	for _, forbidden := range []string{"id", "nickname", "quick_entry", "quickEntry", "permissionCodes", "permission_codes", "button_codes"} {
		if _, ok := meData[forbidden]; ok {
			t.Fatalf("canvas users/me response must not include alias/admin-only field %q: %#v", forbidden, meData)
		}
	}

	logoutRecorder := httptest.NewRecorder()
	logoutRequest := httptest.NewRequest(http.MethodPost, "/api/canvas/v1/auth/logout", nil)
	logoutRequest.Header.Set("Authorization", "Bearer canvas-token")
	router.ServeHTTP(logoutRecorder, logoutRequest)
	if logoutRecorder.Code != http.StatusOK {
		t.Fatalf("expected canvas logout status %d, got %d body=%s", http.StatusOK, logoutRecorder.Code, logoutRecorder.Body.String())
	}
	if logoutToken != "canvas-token" {
		t.Fatalf("expected canvas logout token canvas-token, got %q", logoutToken)
	}
}
func TestRouterInstallsAppProfileAndUploadRoutes(t *testing.T) {
	var tokenInput middleware.TokenInput
	permissionChecked := false
	userService := &fakeRouterUserService{
		profileResult: &user.ProfileResponse{
			Profile: user.ProfileDetail{
				UserID:        7,
				Username:      "移动端用户",
				Email:         "app@example.test",
				Phone:         "15671628271",
				Avatar:        "avatar.png",
				RoleID:        99,
				RoleName:      "管理员",
				Sex:           1,
				Birthday:      "2026-05-24",
				AddressID:     3,
				DetailAddress: "湖北武汉",
				Bio:           "old bio",
				HasPassword:   true,
			},
			Dict: user.ProfileDict{
				SexArr: []user.SexOption{{Label: "男", Value: 1}},
			},
		},
	}
	uploadTokenService := &fakeRouterUploadTokenService{}
	router := newTestRouter(t, testDependencies{
		Authenticator: func(ctx context.Context, input middleware.TokenInput) (*middleware.AuthIdentity, *apperror.Error) {
			tokenInput = input
			return &middleware.AuthIdentity{UserID: 7, SessionID: 20, Platform: input.Platform}, nil
		},
		PermissionRules: map[middleware.RouteKey]string{},
		PermissionChecker: func(ctx context.Context, input middleware.PermissionInput) *apperror.Error {
			permissionChecked = true
			return nil
		},
		UserService:        userService,
		UploadTokenService: uploadTokenService,
	})

	profileRecorder := httptest.NewRecorder()
	profileRequest := httptest.NewRequest(http.MethodGet, "/api/app/v1/profile", nil)
	profileRequest.Header.Set("Authorization", "Bearer app-token")
	router.ServeHTTP(profileRecorder, profileRequest)
	if profileRecorder.Code != http.StatusOK {
		t.Fatalf("expected app profile status %d, got %d body=%s", http.StatusOK, profileRecorder.Code, profileRecorder.Body.String())
	}
	if tokenInput.AccessToken != "app-token" || tokenInput.Platform != enum.PlatformApp {
		t.Fatalf("expected app profile to authenticate as app, got %#v", tokenInput)
	}
	if userService.profileUserID != 7 || userService.profileViewer != 7 {
		t.Fatalf("unexpected app profile service input: user=%d viewer=%d", userService.profileUserID, userService.profileViewer)
	}
	profileData := mustRouterData(t, decodeRouterBody(t, profileRecorder))
	profile, ok := profileData["profile"].(map[string]any)
	if !ok || profile["nickname"] != "移动端用户" || profile["avatar"] != "avatar.png" || profile["bio"] != "old bio" {
		t.Fatalf("unexpected app profile payload: %#v", profileData)
	}
	if _, ok := profile["role_id"]; ok {
		t.Fatalf("app profile must not leak admin role fields: %#v", profile)
	}
	if _, ok := profile["role_name"]; ok {
		t.Fatalf("app profile must not leak admin role fields: %#v", profile)
	}

	updateRecorder := httptest.NewRecorder()
	updateRequest := httptest.NewRequest(http.MethodPut, "/api/app/v1/profile", strings.NewReader(`{"nickname":"移动端用户2","avatar":"avatar2.png","sex":2,"birthday":"2026-05-25","address_id":8,"detail_address":"湖北武汉光谷","bio":"new bio"}`))
	updateRequest.Header.Set("Authorization", "Bearer app-token")
	updateRequest.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(updateRecorder, updateRequest)
	if updateRecorder.Code != http.StatusOK {
		t.Fatalf("expected app profile update status %d, got %d body=%s", http.StatusOK, updateRecorder.Code, updateRecorder.Body.String())
	}
	if userService.profileUpdate.UserID != 7 ||
		userService.profileUpdate.Username != "移动端用户2" ||
		userService.profileUpdate.Avatar != "avatar2.png" ||
		userService.profileUpdate.Sex != 2 ||
		userService.profileUpdate.AddressID != 8 ||
		userService.profileUpdate.DetailAddress != "湖北武汉光谷" ||
		userService.profileUpdate.Bio != "new bio" ||
		userService.profileUpdate.Birthday == nil ||
		*userService.profileUpdate.Birthday != "2026-05-25" {
		t.Fatalf("unexpected app profile update input: %#v", userService.profileUpdate)
	}

	uploadRecorder := httptest.NewRecorder()
	uploadRequest := httptest.NewRequest(http.MethodPost, "/api/app/v1/upload-tokens", strings.NewReader(`{"folder":"avatars","file_name":"avatar.png","file_size":1024,"file_kind":"image"}`))
	uploadRequest.Header.Set("Authorization", "Bearer app-token")
	uploadRequest.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(uploadRecorder, uploadRequest)
	if uploadRecorder.Code != http.StatusOK {
		t.Fatalf("expected app upload token status %d, got %d body=%s", http.StatusOK, uploadRecorder.Code, uploadRecorder.Body.String())
	}
	if permissionChecked {
		t.Fatalf("app profile/upload routes must not run admin RBAC permission checker")
	}
	if uploadTokenService.input.Folder != "avatars" || uploadTokenService.input.FileName != "avatar.png" || uploadTokenService.input.FileSize != 1024 || uploadTokenService.input.FileKind != "image" {
		t.Fatalf("unexpected app upload token input: %#v", uploadTokenService.input)
	}
	uploadData := mustRouterData(t, decodeRouterBody(t, uploadRecorder))
	if uploadData["provider"] != "cos" {
		t.Fatalf("expected app upload token cos response, got %#v", uploadData)
	}
}

func TestRouterDoesNotInstallUsersInitBootstrapRoute(t *testing.T) {
	router := newTestRouter(t, testDependencies{
		Authenticator: func(ctx context.Context, input middleware.TokenInput) (*middleware.AuthIdentity, *apperror.Error) {
			return &middleware.AuthIdentity{UserID: 1, SessionID: 10, Platform: input.Platform}, nil
		},
		UserService: &fakeRouterUserService{result: &user.InitResponse{UserID: 1, Username: "admin"}},
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/users/init", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	request.Header.Set("platform", "admin")
	request.Header.Set("device-id", "desktop-1")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("users/init must not be mounted, got %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestLegacyUsersRoutesAreNotRegistered(t *testing.T) {
	router := newTestRouter(t, testDependencies{
		Authenticator: func(ctx context.Context, input middleware.TokenInput) (*middleware.AuthIdentity, *apperror.Error) {
			return &middleware.AuthIdentity{UserID: 1, SessionID: 10, Platform: "admin"}, nil
		},
		UserService: &fakeRouterUserService{result: &user.InitResponse{UserID: 1, Username: "admin"}},
		AuthService: fakeAuthService{},
	})

	legacyUsersPrefix := "/api/" + "Users"
	for _, tt := range []struct {
		method string
		path   string
	}{
		{http.MethodPost, legacyUsersPrefix + "/getLoginConfig"},
		{http.MethodPost, legacyUsersPrefix + "/sendCode"},
		{http.MethodPost, legacyUsersPrefix + "/login"},
		{http.MethodPost, legacyUsersPrefix + "/refresh"},
		{http.MethodPost, legacyUsersPrefix + "/logout"},
		{http.MethodPost, legacyUsersPrefix + "/init"},
	} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(tt.method, tt.path, nil)
		request.Header.Set("Authorization", "Bearer access-token")
		request.Header.Set("platform", "admin")
		router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("legacy Users route %s %s must not be installed, got code=%d body=%s", tt.method, tt.path, recorder.Code, recorder.Body.String())
		}
	}
}

func TestRouterInstallsUserManagementRESTRoutes(t *testing.T) {
	userService := &fakeRouterUserService{}
	router := newTestRouter(t, testDependencies{
		Authenticator: func(ctx context.Context, input middleware.TokenInput) (*middleware.AuthIdentity, *apperror.Error) {
			return &middleware.AuthIdentity{UserID: 1, SessionID: 10, Platform: "admin"}, nil
		},
		UserService: userService,
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/users?current_page=1&page_size=20&username=admin", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if userService.listQuery.CurrentPage != 1 || userService.listQuery.PageSize != 20 || userService.listQuery.Username != "admin" {
		t.Fatalf("user list query mismatch: %#v", userService.listQuery)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/admin/v1/users/page-init", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !userService.pageInitCalled {
		t.Fatalf("expected users page-init route, code=%d body=%s called=%v", recorder.Code, recorder.Body.String(), userService.pageInitCalled)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/admin/v1/profile", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || userService.profileUserID != 1 || userService.profileViewer != 1 {
		t.Fatalf("expected current profile route, code=%d body=%s service=%#v", recorder.Code, recorder.Body.String(), userService)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/admin/v1/users/9/profile", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || userService.profileUserID != 9 || userService.profileViewer != 1 {
		t.Fatalf("expected target profile route, code=%d body=%s service=%#v", recorder.Code, recorder.Body.String(), userService)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/admin/v1/users/export", strings.NewReader(`{"ids":[3,2]}`))
	request.Header.Set("Authorization", "Bearer access-token")
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || userService.exportInput.UserID != 1 || userService.exportInput.Platform != "admin" || !reflect.DeepEqual(userService.exportInput.IDs, []int64{3, 2}) {
		t.Fatalf("expected user export route, code=%d body=%s input=%#v", recorder.Code, recorder.Body.String(), userService.exportInput)
	}
}

func TestRouterInstallsUserSessionReadOnlyRESTRoutes(t *testing.T) {
	sessionAdminService := &fakeRouterSessionAdminService{}
	router := newTestRouter(t, testDependencies{
		Authenticator: func(ctx context.Context, input middleware.TokenInput) (*middleware.AuthIdentity, *apperror.Error) {
			return &middleware.AuthIdentity{UserID: 1, SessionID: 10, Platform: "admin"}, nil
		},
		SessionAdminService: sessionAdminService,
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/user-sessions/page-init", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected user session page-init status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/admin/v1/user-sessions?current_page=2&page_size=30&username=test&platform=admin&status=active", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected user session list status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	query := sessionAdminService.listQuery
	if query.CurrentPage != 2 || query.PageSize != 30 || query.Username != "test" || query.Platform != "admin" || query.Status != "active" {
		t.Fatalf("user session list query mismatch: %#v", query)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/admin/v1/user-sessions/stats", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected user session stats status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
}

func TestRouterInstallsUserLegacyClosureRESTRoutes(t *testing.T) {
	loginLogService := &fakeRouterLoginLogService{}
	sessionAdminService := &fakeRouterSessionAdminService{}
	router := newTestRouter(t, testDependencies{
		Authenticator: func(ctx context.Context, input middleware.TokenInput) (*middleware.AuthIdentity, *apperror.Error) {
			return &middleware.AuthIdentity{UserID: 44, SessionID: 55, Platform: "admin"}, nil
		},
		LoginLogService:     loginLogService,
		SessionAdminService: sessionAdminService,
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/api/admin/v1/users/me/quick-entries", strings.NewReader(`{"permission_ids":[3,1,3]}`))
	request.Header.Set("Authorization", "Bearer access-token")
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("quick-entry route must not be mounted, got %d body=%s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/admin/v1/users/login-logs/page-init", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected login-log page-init route, code=%d body=%s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/admin/v1/users/login-logs?current_page=2&page_size=30&login_account=adm&date_start=2026-05-01&date_end=2026-05-08", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || loginLogService.listQuery.CurrentPage != 2 || loginLogService.listQuery.LoginAccount != "adm" || loginLogService.listQuery.DateEnd != "2026-05-08" {
		t.Fatalf("expected login-log list route, code=%d body=%s query=%#v", recorder.Code, recorder.Body.String(), loginLogService.listQuery)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPatch, "/api/admin/v1/user-sessions/77/revoke", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || sessionAdminService.revokeID != 77 || sessionAdminService.currentSession != 55 {
		t.Fatalf("expected session revoke route, code=%d body=%s service=%#v", recorder.Code, recorder.Body.String(), sessionAdminService)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPatch, "/api/admin/v1/user-sessions/revoke", strings.NewReader(`{"ids":[77,78]}`))
	request.Header.Set("Authorization", "Bearer access-token")
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || sessionAdminService.currentSession != 55 || !reflect.DeepEqual(sessionAdminService.batchInput.IDs, []int64{77, 78}) {
		t.Fatalf("expected session batch revoke route, code=%d body=%s service=%#v", recorder.Code, recorder.Body.String(), sessionAdminService)
	}
}

func TestRouterInstallsExportTaskRESTRoutes(t *testing.T) {
	exportTaskService := &fakeRouterExportTaskService{}
	router := newTestRouter(t, testDependencies{
		Authenticator: func(ctx context.Context, input middleware.TokenInput) (*middleware.AuthIdentity, *apperror.Error) {
			return &middleware.AuthIdentity{UserID: 12, SessionID: 10, Platform: "admin"}, nil
		},
		ExportTaskService: exportTaskService,
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/export-tasks/status-count?title=%E7%94%A8%E6%88%B7", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || exportTaskService.statusQuery.UserID != 12 || exportTaskService.statusQuery.Title != "用户" {
		t.Fatalf("expected export task status-count route, code=%d body=%s query=%#v", recorder.Code, recorder.Body.String(), exportTaskService.statusQuery)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/admin/v1/export-tasks?current_page=1&page_size=20&status=2", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || exportTaskService.listQuery.UserID != 12 || exportTaskService.listQuery.Status == nil || *exportTaskService.listQuery.Status != 2 {
		t.Fatalf("expected export task list route, code=%d body=%s query=%#v", recorder.Code, recorder.Body.String(), exportTaskService.listQuery)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodDelete, "/api/admin/v1/export-tasks/7", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || exportTaskService.deleteInput.UserID != 12 || !reflect.DeepEqual(exportTaskService.deleteInput.IDs, []int64{7}) {
		t.Fatalf("expected export task delete route, code=%d body=%s input=%#v", recorder.Code, recorder.Body.String(), exportTaskService.deleteInput)
	}
}

func TestRouterInstallsNotificationListAsCurrentUserRESTPath(t *testing.T) {
	notificationService := &fakeRouterNotificationService{}
	var authInput middleware.TokenInput
	router := newTestRouter(t, testDependencies{
		Authenticator: func(ctx context.Context, input middleware.TokenInput) (*middleware.AuthIdentity, *apperror.Error) {
			authInput = input
			return &middleware.AuthIdentity{UserID: 12, SessionID: 10, Platform: input.Platform}, nil
		},
		NotificationService: notificationService,
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/notifications?current_page=1&page_size=5&keyword=%E5%AF%BC%E5%87%BA&type=2&level=2&is_read=2", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	request.Header.Set("platform", "admin")
	request.Header.Set("device-id", "desktop-1")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected notification list status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if authInput.AccessToken != "access-token" || authInput.Platform != "admin" || authInput.DeviceID != "desktop-1" {
		t.Fatalf("unexpected auth input: %#v", authInput)
	}
	query := notificationService.listQuery
	if query.UserID != 12 || query.Platform != "admin" || query.CurrentPage != 1 || query.PageSize != 5 || query.Keyword != "导出" {
		t.Fatalf("notification list query mismatch: %#v", query)
	}
	if query.Type == nil || *query.Type != 2 || query.Level == nil || *query.Level != 2 || query.IsRead == nil || *query.IsRead != 2 {
		t.Fatalf("notification list filters mismatch: %#v", query)
	}
	body := decodeRouterBody(t, recorder)
	data := mustRouterData(t, body)
	if _, ok := data["list"]; !ok {
		t.Fatalf("missing notification list in response: %#v", data)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/admin/v1/notifications/unread-count", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	request.Header.Set("platform", "admin")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected notification unread-count status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if notificationService.unreadIdentity.UserID != 12 || notificationService.unreadIdentity.Platform != "admin" {
		t.Fatalf("notification unread identity mismatch: %#v", notificationService.unreadIdentity)
	}
}

func TestRouterInstallsNotificationReadAndDeleteRoutes(t *testing.T) {
	notificationService := &fakeRouterNotificationService{}
	router := newTestRouter(t, testDependencies{
		Authenticator: func(ctx context.Context, input middleware.TokenInput) (*middleware.AuthIdentity, *apperror.Error) {
			return &middleware.AuthIdentity{UserID: 12, SessionID: 10, Platform: "admin"}, nil
		},
		NotificationService: notificationService,
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPatch, "/api/admin/v1/notifications/7/read", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected mark-one-read status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if notificationService.markIdentity.UserID != 12 || notificationService.markIdentity.Platform != "admin" || !reflect.DeepEqual(notificationService.markIDs, []int64{7}) {
		t.Fatalf("notification mark-one-read mismatch: identity=%#v ids=%#v", notificationService.markIdentity, notificationService.markIDs)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPatch, "/api/admin/v1/notifications/read", strings.NewReader(`{"ids":[3,4]}`))
	request.Header.Set("Authorization", "Bearer access-token")
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected mark-batch-read status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if !reflect.DeepEqual(notificationService.markIDs, []int64{3, 4}) {
		t.Fatalf("notification mark-batch-read ids mismatch: %#v", notificationService.markIDs)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPatch, "/api/admin/v1/notifications/read", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected mark-all-read status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if len(notificationService.markIDs) != 0 {
		t.Fatalf("notification mark-all-read must pass empty ids, got %#v", notificationService.markIDs)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodDelete, "/api/admin/v1/notifications/9", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected delete-one status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if notificationService.deleteIdentity.UserID != 12 || notificationService.deleteIdentity.Platform != "admin" || !reflect.DeepEqual(notificationService.deleteIDs, []int64{9}) {
		t.Fatalf("notification delete-one mismatch: identity=%#v ids=%#v", notificationService.deleteIdentity, notificationService.deleteIDs)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodDelete, "/api/admin/v1/notifications", strings.NewReader(`{"ids":[1,2]}`))
	request.Header.Set("Authorization", "Bearer access-token")
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected delete-batch status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if !reflect.DeepEqual(notificationService.deleteIDs, []int64{1, 2}) {
		t.Fatalf("notification delete-batch ids mismatch: %#v", notificationService.deleteIDs)
	}
}

func TestRouterInstallsNotificationTaskRESTRoutes(t *testing.T) {
	notificationTaskService := &fakeRouterNotificationTaskService{}
	router := newTestRouter(t, testDependencies{
		Authenticator: func(ctx context.Context, input middleware.TokenInput) (*middleware.AuthIdentity, *apperror.Error) {
			return &middleware.AuthIdentity{UserID: 12, SessionID: 10, Platform: "admin"}, nil
		},
		NotificationTaskService: notificationTaskService,
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/notification-tasks/page-init", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected notification task page-init status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/admin/v1/notification-tasks/status-count?title=%E5%8F%91%E5%B8%83", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || notificationTaskService.statusCountQuery.Title != "发布" {
		t.Fatalf("expected notification task status-count route, code=%d body=%s query=%#v", recorder.Code, recorder.Body.String(), notificationTaskService.statusCountQuery)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/admin/v1/notification-tasks?current_page=2&page_size=10&status=1&title=%E9%80%9A%E7%9F%A5", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected notification task list status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if notificationTaskService.listQuery.CurrentPage != 2 || notificationTaskService.listQuery.PageSize != 10 || notificationTaskService.listQuery.Title != "通知" {
		t.Fatalf("notification task list query mismatch: %#v", notificationTaskService.listQuery)
	}
	if notificationTaskService.listQuery.Status == nil || *notificationTaskService.listQuery.Status != 1 {
		t.Fatalf("notification task list status mismatch: %#v", notificationTaskService.listQuery.Status)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/admin/v1/notification-tasks", strings.NewReader(`{"title":"发布通知","target_type":2,"target_ids":[3,4],"platform":"admin"}`))
	request.Header.Set("Authorization", "Bearer access-token")
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected notification task create status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if notificationTaskService.createInput.CreatedBy != 12 || notificationTaskService.createInput.Title != "发布通知" || notificationTaskService.createInput.Platform != "admin" {
		t.Fatalf("notification task create input mismatch: %#v", notificationTaskService.createInput)
	}
	if !reflect.DeepEqual(notificationTaskService.createInput.TargetIDs, []int64{3, 4}) {
		t.Fatalf("notification task create target ids mismatch: %#v", notificationTaskService.createInput.TargetIDs)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPatch, "/api/admin/v1/notification-tasks/7/cancel", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || notificationTaskService.cancelID != 7 {
		t.Fatalf("expected notification task cancel route, code=%d body=%s id=%d", recorder.Code, recorder.Body.String(), notificationTaskService.cancelID)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodDelete, "/api/admin/v1/notification-tasks/8", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || notificationTaskService.deleteID != 8 {
		t.Fatalf("expected notification task delete route, code=%d body=%s id=%d", recorder.Code, recorder.Body.String(), notificationTaskService.deleteID)
	}
}

func TestRouterInstallsPermissionRESTRoutes(t *testing.T) {
	permissionService := &fakeRouterPermissionService{}
	router := newTestRouter(t, testDependencies{
		Authenticator: func(ctx context.Context, input middleware.TokenInput) (*middleware.AuthIdentity, *apperror.Error) {
			return &middleware.AuthIdentity{UserID: 1, SessionID: 10, Platform: "admin"}, nil
		},
		PermissionService: permissionService,
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/permissions?platform=admin", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if permissionService.listQuery.Platform != "admin" {
		t.Fatalf("permission list query mismatch: %#v", permissionService.listQuery)
	}
}

func TestRouterInstallsRoleRESTRoutes(t *testing.T) {
	roleService := &fakeRouterRoleService{}
	router := newTestRouter(t, testDependencies{
		Authenticator: func(ctx context.Context, input middleware.TokenInput) (*middleware.AuthIdentity, *apperror.Error) {
			return &middleware.AuthIdentity{UserID: 1, SessionID: 10, Platform: "admin"}, nil
		},
		RoleService: roleService,
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/roles?current_page=1&page_size=50&name=管理", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if roleService.listQuery.CurrentPage != 1 || roleService.listQuery.PageSize != 50 || roleService.listQuery.Name != "管理" {
		t.Fatalf("role list query mismatch: %#v", roleService.listQuery)
	}
}

func TestRouterInstallsAuthPlatformRESTRoutes(t *testing.T) {
	authPlatformService := &fakeRouterAuthPlatformService{}
	router := newTestRouter(t, testDependencies{
		Authenticator: func(ctx context.Context, input middleware.TokenInput) (*middleware.AuthIdentity, *apperror.Error) {
			return &middleware.AuthIdentity{UserID: 1, SessionID: 10, Platform: "admin"}, nil
		},
		AuthPlatformService: authPlatformService,
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/auth-platforms?current_page=1&page_size=50&status=1", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if authPlatformService.listQuery.CurrentPage != 1 || authPlatformService.listQuery.PageSize != 50 || authPlatformService.listQuery.Status == nil || *authPlatformService.listQuery.Status != 1 {
		t.Fatalf("auth platform list query mismatch: %#v", authPlatformService.listQuery)
	}
	body := decodeRouterBody(t, recorder)
	data := mustRouterData(t, body)
	if _, ok := data["list"]; !ok {
		t.Fatalf("missing list in auth-platforms response: %#v", data)
	}
}

func TestRouterInstallsClientVersionRESTRoutes(t *testing.T) {
	clientVersionService := &fakeRouterClientVersionService{}
	var permissionInputs []middleware.PermissionInput
	var authCalls int
	router := newTestRouter(t, testDependencies{
		Authenticator: func(ctx context.Context, input middleware.TokenInput) (*middleware.AuthIdentity, *apperror.Error) {
			authCalls++
			return &middleware.AuthIdentity{UserID: 1, SessionID: 10, Platform: "admin"}, nil
		},
		PermissionRules: map[middleware.RouteKey]string{
			middleware.NewRouteKey(http.MethodPost, "/api/admin/v1/client-versions"):                   "system_clientVersion_add",
			middleware.NewRouteKey(http.MethodPut, "/api/admin/v1/client-versions/:id"):                "system_clientVersion_edit",
			middleware.NewRouteKey(http.MethodPatch, "/api/admin/v1/client-versions/:id/latest"):       "system_clientVersion_setLatest",
			middleware.NewRouteKey(http.MethodPatch, "/api/admin/v1/client-versions/:id/force-update"): "system_clientVersion_forceUpdate",
			middleware.NewRouteKey(http.MethodDelete, "/api/admin/v1/client-versions/:id"):             "system_clientVersion_del",
		},
		PermissionChecker: func(ctx context.Context, input middleware.PermissionInput) *apperror.Error {
			permissionInputs = append(permissionInputs, input)
			return nil
		},
		ClientVersionService: clientVersionService,
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/client-versions/page-init", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !clientVersionService.initCalled {
		t.Fatalf("expected client version init route, code=%d body=%s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/admin/v1/client-versions?current_page=1&page_size=20&platform=windows-x86_64", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected client version list status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if clientVersionService.listQuery.CurrentPage != 1 || clientVersionService.listQuery.PageSize != 20 || clientVersionService.listQuery.Platform != enum.ClientPlatformWindowsX8664 {
		t.Fatalf("client version list query mismatch: %#v", clientVersionService.listQuery)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/admin/v1/client-versions", strings.NewReader(`{"version":"1.0.8","notes":"release","file_url":"https://example.com/app.exe","signature":"sig","platform":"windows-x86_64","file_size":128,"force_update":2}`))
	request.Header.Set("Authorization", "Bearer access-token")
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || clientVersionService.createInput.Version != "1.0.8" || clientVersionService.createInput.Platform != enum.ClientPlatformWindowsX8664 {
		t.Fatalf("expected client version create route, code=%d body=%s input=%#v", recorder.Code, recorder.Body.String(), clientVersionService.createInput)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPut, "/api/admin/v1/client-versions/8", strings.NewReader(`{"version":"1.0.8","notes":"release-2","file_url":"https://example.com/app.exe","signature":"sig2","platform":"windows-x86_64","file_size":256,"force_update":1}`))
	request.Header.Set("Authorization", "Bearer access-token")
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || clientVersionService.updateID != 8 || clientVersionService.updateInput.ForceUpdate != enum.CommonYes {
		t.Fatalf("expected client version update route, code=%d body=%s id=%d input=%#v", recorder.Code, recorder.Body.String(), clientVersionService.updateID, clientVersionService.updateInput)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPatch, "/api/admin/v1/client-versions/8/latest", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || clientVersionService.latestID != 8 {
		t.Fatalf("expected client version latest route, code=%d body=%s id=%d", recorder.Code, recorder.Body.String(), clientVersionService.latestID)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPatch, "/api/admin/v1/client-versions/8/force-update", strings.NewReader(`{"force_update":1}`))
	request.Header.Set("Authorization", "Bearer access-token")
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || clientVersionService.forceID != 8 || clientVersionService.forceUpdate != enum.CommonYes {
		t.Fatalf("expected client version force-update route, code=%d body=%s id=%d force=%d", recorder.Code, recorder.Body.String(), clientVersionService.forceID, clientVersionService.forceUpdate)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodDelete, "/api/admin/v1/client-versions/8", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || clientVersionService.deleteID != 8 {
		t.Fatalf("expected client version delete route, code=%d body=%s id=%d", recorder.Code, recorder.Body.String(), clientVersionService.deleteID)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/admin/v1/client-versions/update-json?platform=windows-x86_64", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || clientVersionService.updateJSONPlatform != enum.ClientPlatformWindowsX8664 {
		t.Fatalf("expected client version update-json route, code=%d body=%s platform=%q", recorder.Code, recorder.Body.String(), clientVersionService.updateJSONPlatform)
	}

	authCallsBeforePublic := authCalls
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/admin/v1/client-versions/current-check?version=1.0.7&platform=windows-x86_64", nil)
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected public current-check status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if clientVersionService.currentCheckQuery.Version != "1.0.7" || clientVersionService.currentCheckQuery.Platform != enum.ClientPlatformWindowsX8664 {
		t.Fatalf("client version current-check query mismatch: %#v", clientVersionService.currentCheckQuery)
	}
	if authCalls != authCallsBeforePublic {
		t.Fatalf("public current-check must not call authenticator: before=%d after=%d", authCallsBeforePublic, authCalls)
	}

	gotCodes := make([]string, 0, len(permissionInputs))
	for _, input := range permissionInputs {
		gotCodes = append(gotCodes, input.Code)
	}
	wantCodes := []string{
		"system_clientVersion_add",
		"system_clientVersion_edit",
		"system_clientVersion_setLatest",
		"system_clientVersion_forceUpdate",
		"system_clientVersion_del",
	}
	if !reflect.DeepEqual(gotCodes, wantCodes) {
		t.Fatalf("client version permission codes mismatch: got=%#v want=%#v", gotCodes, wantCodes)
	}
}

func TestRouterInstallsAIConfigRESTRoutes(t *testing.T) {
	providerService := &fakeRouterAIProviderService{}
	agentService := &fakeRouterAIAgentService{}
	toolService := &fakeRouterAIToolService{}
	router := newTestRouter(t, testDependencies{
		Authenticator: func(ctx context.Context, input middleware.TokenInput) (*middleware.AuthIdentity, *apperror.Error) {
			return &middleware.AuthIdentity{UserID: 9, SessionID: 10, Platform: "admin"}, nil
		},
		AiProviderService: providerService,
		AiAgentService:    agentService,
		AiToolService:     toolService,
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/ai-providers/page-init", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !providerService.initCalled {
		t.Fatalf("expected AI provider page-init route, code=%d body=%s called=%v", recorder.Code, recorder.Body.String(), providerService.initCalled)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/admin/v1/ai-providers?current_page=1&page_size=20&engine_type=openai&status=1", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || providerService.listQuery.EngineType != "openai" || providerService.listQuery.Status == nil || *providerService.listQuery.Status != enum.CommonYes {
		t.Fatalf("expected AI provider list route, code=%d body=%s query=%#v", recorder.Code, recorder.Body.String(), providerService.listQuery)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/admin/v1/ai-providers/model-options", strings.NewReader(`{"engine_type":"openai","api_key":"sk-test"}`))
	request.Header.Set("Authorization", "Bearer access-token")
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !providerService.previewCalled {
		t.Fatalf("expected AI provider model-options route, code=%d body=%s called=%v", recorder.Code, recorder.Body.String(), providerService.previewCalled)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/admin/v1/ai-providers/7/model-options", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || providerService.storedPreviewID != 7 {
		t.Fatalf("expected AI provider stored model-options route, code=%d body=%s id=%d", recorder.Code, recorder.Body.String(), providerService.storedPreviewID)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/admin/v1/ai-providers/7/test", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || providerService.testID != 7 {
		t.Fatalf("expected AI provider test route, code=%d body=%s id=%d", recorder.Code, recorder.Body.String(), providerService.testID)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/admin/v1/ai-providers/7/sync-models", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || providerService.syncID != 7 {
		t.Fatalf("expected AI provider sync-models route, code=%d body=%s id=%d", recorder.Code, recorder.Body.String(), providerService.syncID)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/admin/v1/ai-providers/7/models", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || providerService.modelsID != 7 {
		t.Fatalf("expected AI provider models route, code=%d body=%s id=%d", recorder.Code, recorder.Body.String(), providerService.modelsID)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPut, "/api/admin/v1/ai-providers/7/models", strings.NewReader(`{"model_ids":["gpt-4.1-mini"]}`))
	request.Header.Set("Authorization", "Bearer access-token")
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || providerService.updateModelsID != 7 || len(providerService.updateModelsBody.ModelIDs) != 1 || providerService.updateModelsBody.ModelIDs[0] != "gpt-4.1-mini" {
		t.Fatalf("expected AI provider update models route, code=%d body=%s id=%d input=%#v", recorder.Code, recorder.Body.String(), providerService.updateModelsID, providerService.updateModelsBody)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/admin/v1/ai-agents/page-init", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !agentService.initCalled {
		t.Fatalf("expected AI agent page-init route, code=%d body=%s called=%v", recorder.Code, recorder.Body.String(), agentService.initCalled)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/admin/v1/ai-agents?current_page=2&page_size=10&scene=chat&provider_id=3&status=1", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || agentService.listQuery.Scene != "chat" || agentService.listQuery.ProviderID != 3 || agentService.listQuery.Status == nil || *agentService.listQuery.Status != enum.CommonYes {
		t.Fatalf("expected AI agent list route, code=%d body=%s query=%#v", recorder.Code, recorder.Body.String(), agentService.listQuery)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/admin/v1/ai-agents/provider-models/3", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || agentService.providerModelsID != 3 {
		t.Fatalf("expected AI agent provider-models route, code=%d body=%s providerID=%d", recorder.Code, recorder.Body.String(), agentService.providerModelsID)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/admin/v1/ai-agents/options", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || agentService.optionQuery.UserID != 9 {
		t.Fatalf("expected AI agent options route scoped to auth identity, code=%d body=%s query=%#v", recorder.Code, recorder.Body.String(), agentService.optionQuery)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/admin/v1/ai-agents/5", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || agentService.detailID != 5 {
		t.Fatalf("expected AI agent detail route, code=%d body=%s id=%d", recorder.Code, recorder.Body.String(), agentService.detailID)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/admin/v1/ai-agents/5/test", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || agentService.testID != 5 {
		t.Fatalf("expected AI agent test route, code=%d body=%s id=%d", recorder.Code, recorder.Body.String(), agentService.testID)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/admin/v1/ai-tools/page-init", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !toolService.initCalled {
		t.Fatalf("expected AI tool page-init route, code=%d body=%s called=%v", recorder.Code, recorder.Body.String(), toolService.initCalled)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/admin/v1/ai-tools/generate/page-init", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !toolService.generateInit {
		t.Fatalf("expected AI tool generate page-init route, code=%d body=%s called=%v", recorder.Code, recorder.Body.String(), toolService.generateInit)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/admin/v1/ai-tools/generate-draft", strings.NewReader(`{"agent_id":5,"requirement":"生成查询当前用户量工具","code_hint":"admin_user_count"}`))
	request.Header.Set("Authorization", "Bearer access-token")
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || toolService.generateInput.AgentID != 5 || toolService.generateInput.UserID != 9 || toolService.generateInput.CodeHint != "admin_user_count" {
		t.Fatalf("expected AI tool generate-draft route, code=%d body=%s input=%#v", recorder.Code, recorder.Body.String(), toolService.generateInput)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/admin/v1/ai-tools?current_page=2&page_size=10&name=查询&code=admin_user_count&risk_level=low&status=1", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || toolService.listQuery.Name != "查询" || toolService.listQuery.Code != "admin_user_count" || toolService.listQuery.RiskLevel != aitool.RiskLow || toolService.listQuery.Status == nil || *toolService.listQuery.Status != enum.CommonYes {
		t.Fatalf("expected AI tool list route, code=%d body=%s query=%#v", recorder.Code, recorder.Body.String(), toolService.listQuery)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/admin/v1/ai-tools", strings.NewReader(`{"name":"查询当前用户量","code":"admin_user_count","description":"查询数量","parameters_json":{"type":"object","properties":{},"additionalProperties":false},"result_schema_json":{"type":"object","properties":{},"additionalProperties":false},"risk_level":"low","timeout_ms":3000,"status":1}`))
	request.Header.Set("Authorization", "Bearer access-token")
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected AI tool create route, code=%d body=%s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPut, "/api/admin/v1/ai-tools/4", strings.NewReader(`{"name":"查询当前用户量","code":"admin_user_count","description":"查询数量","parameters_json":{"type":"object","properties":{},"additionalProperties":false},"result_schema_json":{"type":"object","properties":{},"additionalProperties":false},"risk_level":"low","timeout_ms":3000,"status":1}`))
	request.Header.Set("Authorization", "Bearer access-token")
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || toolService.updatedID != 4 {
		t.Fatalf("expected AI tool update route, code=%d body=%s id=%d", recorder.Code, recorder.Body.String(), toolService.updatedID)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPatch, "/api/admin/v1/ai-tools/4/status", strings.NewReader(`{"status":2}`))
	request.Header.Set("Authorization", "Bearer access-token")
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || toolService.statusID != 4 {
		t.Fatalf("expected AI tool status route, code=%d body=%s id=%d", recorder.Code, recorder.Body.String(), toolService.statusID)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodDelete, "/api/admin/v1/ai-tools/4", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || toolService.deletedID != 4 {
		t.Fatalf("expected AI tool delete route, code=%d body=%s id=%d", recorder.Code, recorder.Body.String(), toolService.deletedID)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/admin/v1/ai-agents/3/tools", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || toolService.bindingID != 3 {
		t.Fatalf("expected AI tool agent binding read route, code=%d body=%s id=%d", recorder.Code, recorder.Body.String(), toolService.bindingID)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPut, "/api/admin/v1/ai-agents/3/tools", strings.NewReader(`{"tool_ids":[1]}`))
	request.Header.Set("Authorization", "Bearer access-token")
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || toolService.bindingID != 3 || len(toolService.bindingToolID) != 1 || toolService.bindingToolID[0] != 1 {
		t.Fatalf("expected AI tool agent binding update route, code=%d body=%s id=%d tools=%#v", recorder.Code, recorder.Body.String(), toolService.bindingID, toolService.bindingToolID)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/admin/v1/ai-tools/agent-options", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)
	if recorder.Code == http.StatusOK {
		t.Fatalf("tool management must not expose agent option route, code=%d body=%s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/admin/v1/ai-tools/agent-bindings/3", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)
	if recorder.Code == http.StatusOK {
		t.Fatalf("tool management must not expose agent binding route, code=%d body=%s", recorder.Code, recorder.Body.String())
	}

}

func TestRouterInstallsOperationLogRESTRoutes(t *testing.T) {
	operationLogService := &fakeRouterOperationLogService{}
	router := newTestRouter(t, testDependencies{
		Authenticator: func(ctx context.Context, input middleware.TokenInput) (*middleware.AuthIdentity, *apperror.Error) {
			return &middleware.AuthIdentity{UserID: 1, SessionID: 10, Platform: "admin"}, nil
		},
		PermissionRules: map[middleware.RouteKey]string{
			middleware.NewRouteKey(http.MethodDelete, "/api/admin/v1/operation-logs/:id"): "devTools_operationLog_del",
		},
		PermissionChecker: func(ctx context.Context, input middleware.PermissionInput) *apperror.Error {
			return nil
		},
		OperationLogService: operationLogService,
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/operation-logs?current_page=1&page_size=20&action=编辑&date=2026-05-01,2026-05-04", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if operationLogService.listQuery.CurrentPage != 1 || operationLogService.listQuery.PageSize != 20 || operationLogService.listQuery.Action != "编辑" {
		t.Fatalf("operation log list query mismatch: %#v", operationLogService.listQuery)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodDelete, "/api/admin/v1/operation-logs/9", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected delete status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if !reflect.DeepEqual(operationLogService.deleteIDs, []int64{9}) {
		t.Fatalf("operation log delete mismatch: %#v", operationLogService.deleteIDs)
	}
}

func TestRouterInstallsCronTaskRESTRoutes(t *testing.T) {
	cronTaskService := &fakeRouterCronTaskService{}
	router := newTestRouter(t, testDependencies{
		Authenticator: func(ctx context.Context, input middleware.TokenInput) (*middleware.AuthIdentity, *apperror.Error) {
			return &middleware.AuthIdentity{UserID: 1, SessionID: 10, Platform: "admin"}, nil
		},
		PermissionRules: map[middleware.RouteKey]string{
			middleware.NewRouteKey(http.MethodPatch, "/api/admin/v1/cron-tasks/:id/status"): "devTools_cronTask_status",
		},
		PermissionChecker: func(ctx context.Context, input middleware.PermissionInput) *apperror.Error {
			return nil
		},
		CronTaskService: cronTaskService,
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/cron-tasks?current_page=1&page_size=20&status=1&title=通知", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected cron task list status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if cronTaskService.listQuery.CurrentPage != 1 || cronTaskService.listQuery.PageSize != 20 || cronTaskService.listQuery.Title != "通知" {
		t.Fatalf("cron task list query mismatch: %#v", cronTaskService.listQuery)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPatch, "/api/admin/v1/cron-tasks/2/status", strings.NewReader(`{"status":2}`))
	request.Header.Set("Authorization", "Bearer access-token")
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || cronTaskService.statusID != 2 || cronTaskService.status != enum.CommonNo {
		t.Fatalf("cron task status mismatch: code=%d body=%s id=%d status=%d", recorder.Code, recorder.Body.String(), cronTaskService.statusID, cronTaskService.status)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/admin/v1/cron-tasks/2/logs?current_page=1&page_size=20&status=1", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || cronTaskService.logsQuery.TaskID != 2 {
		t.Fatalf("cron task logs mismatch: code=%d body=%s query=%#v", recorder.Code, recorder.Body.String(), cronTaskService.logsQuery)
	}
}

func TestRouterInstallsSystemSettingRESTRoutes(t *testing.T) {
	systemSettingService := &fakeRouterSystemSettingService{}
	router := newTestRouter(t, testDependencies{
		Authenticator: func(ctx context.Context, input middleware.TokenInput) (*middleware.AuthIdentity, *apperror.Error) {
			return &middleware.AuthIdentity{UserID: 1, SessionID: 10, Platform: "admin"}, nil
		},
		PermissionRules: map[middleware.RouteKey]string{
			middleware.NewRouteKey(http.MethodPatch, "/api/admin/v1/system-settings/:id/status"): "system_setting_status",
		},
		PermissionChecker: func(ctx context.Context, input middleware.PermissionInput) *apperror.Error {
			return nil
		},
		SystemSettingService: systemSettingService,
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/system-settings?current_page=1&page_size=20&key=user.&status=1", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected system settings list status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if systemSettingService.listQuery.CurrentPage != 1 || systemSettingService.listQuery.PageSize != 20 || systemSettingService.listQuery.Key != "user." || systemSettingService.listQuery.Status == nil || *systemSettingService.listQuery.Status != 1 {
		t.Fatalf("system setting list query mismatch: %#v", systemSettingService.listQuery)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPatch, "/api/admin/v1/system-settings/2/status", strings.NewReader(`{"status":2}`))
	request.Header.Set("Authorization", "Bearer access-token")
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status change status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if systemSettingService.statusID != 2 || systemSettingService.status != 2 {
		t.Fatalf("system setting status mismatch: id=%d status=%d", systemSettingService.statusID, systemSettingService.status)
	}
}

func TestRouterDoesNotInstallLegacyPayWalletRoutes(t *testing.T) {
	router := newTestRouter(t, testDependencies{
		Authenticator: func(ctx context.Context, input middleware.TokenInput) (*middleware.AuthIdentity, *apperror.Error) {
			return &middleware.AuthIdentity{UserID: 1, SessionID: 10, Platform: "admin"}, nil
		},
	})

	for _, tt := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/admin/v1/pay-channels/page-init"},
		{http.MethodGet, "/api/admin/v1/pay-orders/page-init"},
		{http.MethodGet, "/api/admin/v1/pay-transactions/page-init"},
		{http.MethodGet, "/api/admin/v1/pay-notify-logs/page-init"},
		{http.MethodGet, "/api/admin/v1/wallets/page-init"},
		{http.MethodPost, "/api/pay/notify/alipay"},
		{http.MethodPost, "/api/payment/notify/alipay"},
	} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(tt.method, tt.path, nil)
		request.Header.Set("Authorization", "Bearer access-token")
		router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("legacy route %s %s must not be installed, got code=%d body=%s", tt.method, tt.path, recorder.Code, recorder.Body.String())
		}
	}
}

func TestRouterInstallsPublicPaymentCallbackRoute(t *testing.T) {
	paymentService := &fakeRouterPaymentService{}
	router := newTestRouter(t, testDependencies{
		PaymentService: paymentService,
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/payment/callbacks/alipay", strings.NewReader("notify_id=notify-1&out_trade_no=PAY20260521100000000000&trade_no=202605212200&trade_status=TRADE_SUCCESS&app_id=2026000000000000&total_amount=10.00&sign=signature&sign_type=RSA2"))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected public callback route status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if recorder.Body.String() != "success" {
		t.Fatalf("expected plain success body, got %q", recorder.Body.String())
	}
	if !paymentService.callbackCalled {
		t.Fatalf("expected callback service to be invoked")
	}
	if got := paymentService.callbackInput.Form.Get("out_trade_no"); got != "PAY20260521100000000000" {
		t.Fatalf("expected callback form to be forwarded, got %q", got)
	}
}

func TestRouterInstallsMailRoutes(t *testing.T) {
	mailService := &fakeRouterMailService{}
	router := newTestRouter(t, testDependencies{
		Authenticator: func(ctx context.Context, input middleware.TokenInput) (*middleware.AuthIdentity, *apperror.Error) {
			return &middleware.AuthIdentity{UserID: 1, SessionID: 10, Platform: "admin"}, nil
		},
		PermissionRules: map[middleware.RouteKey]string{
			middleware.NewRouteKey(http.MethodPut, "/api/admin/v1/mail/config"):  "system_mail_configEdit",
			middleware.NewRouteKey(http.MethodDelete, "/api/admin/v1/mail/logs"): "system_mail_logDel",
		},
		PermissionChecker: func(ctx context.Context, input middleware.PermissionInput) *apperror.Error { return nil },
		MailService:       mailService,
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/mail/page-init", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !mailService.initCalled {
		t.Fatalf("mail page-init route mismatch: code=%d body=%s init=%v", recorder.Code, recorder.Body.String(), mailService.initCalled)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPut, "/api/admin/v1/mail/config", strings.NewReader(`{"secret_id":"AKID","secret_key":"SECRET","region":"ap-guangzhou","endpoint":"ses.tencentcloudapi.com","from_email":"noreply@example.com","status":1,"verify_code_ttl_minutes":5}`))
	request.Header.Set("Authorization", "Bearer access-token")
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK ||
		mailService.savedConfig.SecretID != "AKID" ||
		mailService.savedConfig.SecretKey != "SECRET" ||
		mailService.savedConfig.VerifyCodeTTLMinutes != 5 {
		t.Fatalf("mail config route mismatch: code=%d body=%s input=%#v", recorder.Code, recorder.Body.String(), mailService.savedConfig)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodDelete, "/api/admin/v1/mail/logs", strings.NewReader(`{"ids":[1,2]}`))
	request.Header.Set("Authorization", "Bearer access-token")
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !reflect.DeepEqual(mailService.deletedLogs, []uint64{1, 2}) {
		t.Fatalf("mail logs delete route mismatch: code=%d body=%s ids=%#v", recorder.Code, recorder.Body.String(), mailService.deletedLogs)
	}
}

func TestRouterInstallsSmsRoutes(t *testing.T) {
	smsService := &fakeRouterSmsService{}
	router := newTestRouter(t, testDependencies{
		Authenticator: func(ctx context.Context, input middleware.TokenInput) (*middleware.AuthIdentity, *apperror.Error) {
			return &middleware.AuthIdentity{UserID: 1, SessionID: 10, Platform: "admin"}, nil
		},
		PermissionRules: map[middleware.RouteKey]string{
			middleware.NewRouteKey(http.MethodPut, "/api/admin/v1/sms/config"):  "system_sms_configEdit",
			middleware.NewRouteKey(http.MethodDelete, "/api/admin/v1/sms/logs"): "system_sms_logDel",
		},
		PermissionChecker: func(ctx context.Context, input middleware.PermissionInput) *apperror.Error { return nil },
		SmsService:        smsService,
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/sms/page-init", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !smsService.initCalled {
		t.Fatalf("sms page-init route mismatch: code=%d body=%s init=%v", recorder.Code, recorder.Body.String(), smsService.initCalled)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPut, "/api/admin/v1/sms/config", strings.NewReader(`{"secret_id":"AKID","secret_key":"SECRET","sms_sdk_app_id":"1400000000","sign_name":"签名","region":"ap-guangzhou","endpoint":"sms.tencentcloudapi.com","status":1,"verify_code_ttl_minutes":5}`))
	request.Header.Set("Authorization", "Bearer access-token")
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK ||
		smsService.savedConfig.SecretID != "AKID" ||
		smsService.savedConfig.SecretKey != "SECRET" ||
		smsService.savedConfig.SmsSdkAppID != "1400000000" ||
		smsService.savedConfig.VerifyCodeTTLMinutes != 5 {
		t.Fatalf("sms config route mismatch: code=%d body=%s input=%#v", recorder.Code, recorder.Body.String(), smsService.savedConfig)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodDelete, "/api/admin/v1/sms/logs", strings.NewReader(`{"ids":[1,2]}`))
	request.Header.Set("Authorization", "Bearer access-token")
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !reflect.DeepEqual(smsService.deletedLogs, []uint64{1, 2}) {
		t.Fatalf("sms logs delete route mismatch: code=%d body=%s ids=%#v", recorder.Code, recorder.Body.String(), smsService.deletedLogs)
	}
}
func TestRouterInstallsPaymentConfigAndRechargeRoutes(t *testing.T) {
	paymentService := &fakeRouterPaymentService{}
	router := newTestRouter(t, testDependencies{
		Authenticator: func(ctx context.Context, input middleware.TokenInput) (*middleware.AuthIdentity, *apperror.Error) {
			return &middleware.AuthIdentity{UserID: 7, SessionID: 10, Platform: "admin"}, nil
		},
		PermissionRules: map[middleware.RouteKey]string{
			middleware.NewRouteKey(http.MethodGet, "/api/admin/v1/payment/configs"):            "payment_config_list",
			middleware.NewRouteKey(http.MethodPost, "/api/admin/v1/payment/configs"):           "payment_config_add",
			middleware.NewRouteKey(http.MethodPost, "/api/admin/v1/payment/configs/:id/test"):  "payment_config_test",
			middleware.NewRouteKey(http.MethodGet, "/api/admin/v1/payment/recharges"):          "payment_recharge_list",
			middleware.NewRouteKey(http.MethodPost, "/api/admin/v1/payment/recharges"):         "payment_recharge_add",
			middleware.NewRouteKey(http.MethodPost, "/api/admin/v1/payment/recharges/:id/pay"): "payment_recharge_pay",
		},
		PermissionChecker: func(ctx context.Context, input middleware.PermissionInput) *apperror.Error { return nil },
		PaymentService:    paymentService,
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/payment/configs?current_page=2&page_size=30&name=ali&provider=alipay&environment=sandbox&status=1", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected payment config list status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if paymentService.configListQuery.CurrentPage != 2 || paymentService.configListQuery.Provider != "alipay" || paymentService.configListQuery.Environment != "sandbox" || paymentService.configListQuery.Status != 1 {
		t.Fatalf("payment config list query mismatch: %#v", paymentService.configListQuery)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/admin/v1/payment/configs", strings.NewReader(`{"provider":"alipay","code":"alipay_default","name":"支付宝","app_id":"2026000000000000","app_private_key":"KEY","app_cert_path":"runtime/app.crt","platform_cert_path":"runtime/alipay.crt","root_cert_path":"runtime/root.crt","notify_url":"https://example.test/notify","environment":"sandbox","enabled_methods":["web"],"status":2,"remark":""}`))
	request.Header.Set("Authorization", "Bearer access-token")
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || paymentService.createInput.Provider != "alipay" || paymentService.createInput.Code != "alipay_default" || paymentService.createInput.PlatformCertPath == "" || paymentService.createInput.RootCertPath == "" {
		t.Fatalf("expected payment config create route, code=%d body=%s service=%#v", recorder.Code, recorder.Body.String(), paymentService)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/admin/v1/payment/configs/1/test", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || paymentService.testID != 1 {
		t.Fatalf("expected payment config test route, code=%d body=%s service=%#v", recorder.Code, recorder.Body.String(), paymentService)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/admin/v1/payment/recharges?current_page=1&page_size=10&keyword=RCG&status=paying", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || paymentService.rechargeQuery.UserID != 7 || paymentService.rechargeQuery.Status != "paying" {
		t.Fatalf("expected payment recharge list route, code=%d body=%s query=%#v", recorder.Code, recorder.Body.String(), paymentService.rechargeQuery)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/admin/v1/payment/recharges", strings.NewReader(`{"package_code":"recharge_10","pay_method":"web","return_url":"https://example.test/payment/recharge"}`))
	request.Header.Set("Authorization", "Bearer access-token")
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || paymentService.rechargeInput.UserID != 7 || paymentService.rechargeInput.PackageCode != "recharge_10" || paymentService.rechargeInput.ReturnURL == "" {
		t.Fatalf("expected payment recharge create route, code=%d body=%s input=%#v", recorder.Code, recorder.Body.String(), paymentService.rechargeInput)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/admin/v1/payment/recharges/2/pay", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || paymentService.payID != 2 {
		t.Fatalf("expected payment recharge pay route, code=%d body=%s service=%#v", recorder.Code, recorder.Body.String(), paymentService)
	}

	for _, retired := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/admin/v1/payment/channels"},
		{http.MethodGet, "/api/admin/v1/payment/events"},
		{http.MethodGet, "/api/admin/v1/payment/" + "order"},
		{http.MethodGet, "/api/admin/v1/payment/orders"},
		{http.MethodPost, "/api/admin/v1/payment/orders"},
		{http.MethodPost, "/api/admin/v1/payment/orders/1/pay"},
		{http.MethodPost, "/api/admin/v1/payment/orders/1/sync"},
		{http.MethodPatch, "/api/admin/v1/payment/orders/1/close"},
		{http.MethodPost, "/api/admin/v1/payment/recharges/2/sync"},
		{http.MethodPatch, "/api/admin/v1/payment/recharges/2/close"},
		{http.MethodPost, "/api/payment/notify/alipay"},
		{http.MethodPost, "/api/pay/notify/alipay"},
	} {
		recorder = httptest.NewRecorder()
		request = httptest.NewRequest(retired.method, retired.path, nil)
		request.Header.Set("Authorization", "Bearer access-token")
		router.ServeHTTP(recorder, request)
		if recorder.Code == http.StatusOK {
			t.Fatalf("retired payment route still returns OK: %s %s", retired.method, retired.path)
		}
	}
}
func TestRouterInstallsCanvasPromptAndAssetRoutes(t *testing.T) {
	canvasService := &fakeRouterCanvasService{}
	promptService := &fakeRouterAiPromptService{}
	assetService := &fakeRouterAiAssetService{}
	router := newTestRouter(t, testDependencies{
		Authenticator: func(ctx context.Context, input middleware.TokenInput) (*middleware.AuthIdentity, *apperror.Error) {
			return &middleware.AuthIdentity{UserID: 9, SessionID: 10, Platform: input.Platform}, nil
		},
		CanvasService:   canvasService,
		AiPromptService: promptService,
		AiAssetService:  assetService,
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/canvas/v1/settings", nil)
	request.Header.Set("Authorization", "Bearer canvas-token")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || canvasService.settings.UserID != 9 {
		t.Fatalf("expected canvas settings route for token user, code=%d body=%s input=%#v", recorder.Code, recorder.Body.String(), canvasService.settings)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/canvas/v1/prompts?keyword=cat&category=style", nil)
	request.Header.Set("Authorization", "Bearer canvas-token")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || promptService.query.Keyword != "cat" || promptService.query.Category != "style" {
		t.Fatalf("expected canvas prompt route from AI prompt service, code=%d body=%s query=%#v", recorder.Code, recorder.Body.String(), promptService.query)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/canvas/v1/assets?keyword=sky&type=image", nil)
	request.Header.Set("Authorization", "Bearer canvas-token")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || assetService.userID != 9 || assetService.query.Keyword != "sky" || assetService.query.Type != aiasset.AssetTypeImage {
		t.Fatalf("expected canvas asset route from AI asset service, code=%d body=%s user=%d query=%#v", recorder.Code, recorder.Body.String(), assetService.userID, assetService.query)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/canvas/v1/assets", strings.NewReader(`{"slug":"clip","type":"video","title":"Clip"}`))
	request.Header.Set("Authorization", "Bearer canvas-token")
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || assetService.userID != 9 || assetService.createCalls != 1 || assetService.created.Slug != "clip" || assetService.created.Type != aiasset.AssetTypeVideo {
		t.Fatalf("expected canvas asset create from AI asset service, code=%d body=%s user=%d calls=%d input=%#v", recorder.Code, recorder.Body.String(), assetService.userID, assetService.createCalls, assetService.created)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPut, "/api/canvas/v1/assets/7", strings.NewReader(`{"slug":"hero","type":"image","title":"Hero"}`))
	request.Header.Set("Authorization", "Bearer canvas-token")
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || assetService.updateCalls != 1 || assetService.updatedID != 7 || assetService.updated.Type != aiasset.AssetTypeImage {
		t.Fatalf("expected canvas asset update from AI asset service, code=%d body=%s calls=%d id=%d input=%#v", recorder.Code, recorder.Body.String(), assetService.updateCalls, assetService.updatedID, assetService.updated)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodDelete, "/api/canvas/v1/assets/7", nil)
	request.Header.Set("Authorization", "Bearer canvas-token")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || assetService.deleteCalls != 1 || assetService.deletedID != 7 {
		t.Fatalf("expected canvas asset delete from AI asset service, code=%d body=%s calls=%d id=%d", recorder.Code, recorder.Body.String(), assetService.deleteCalls, assetService.deletedID)
	}
}

func TestRouterInstallsAdminAIPromptRoutes(t *testing.T) {
	promptService := &fakeRouterAdminAIPromptService{}
	router := newTestRouter(t, testDependencies{
		Authenticator: func(ctx context.Context, input middleware.TokenInput) (*middleware.AuthIdentity, *apperror.Error) {
			return &middleware.AuthIdentity{UserID: 7, SessionID: 10, Platform: "admin"}, nil
		},
		AiPromptAdminService: promptService,
	})

	requests := []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodGet, "/api/admin/v1/ai-prompts/page-init", ""},
		{http.MethodGet, "/api/admin/v1/ai-prompts?keyword=cat&category=style&status=2", ""},
		{http.MethodGet, "/api/admin/v1/ai-prompts/9", ""},
		{http.MethodPost, "/api/admin/v1/ai-prompts", `{"slug":"cat","title":"Cat","prompt":"draw cat","status":2}`},
		{http.MethodPut, "/api/admin/v1/ai-prompts/7", `{"slug":"cat","title":"Cat","prompt":"draw cat","status":1}`},
		{http.MethodPatch, "/api/admin/v1/ai-prompts/7/status", `{"status":2}`},
		{http.MethodDelete, "/api/admin/v1/ai-prompts/7", ""},
		{http.MethodDelete, "/api/admin/v1/ai-prompts", `{"ids":[3,4]}`},
	}
	for _, tc := range requests {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			var body io.Reader
			if tc.body != "" {
				body = strings.NewReader(tc.body)
			}
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(tc.method, tc.path, body)
			request.Header.Set("Authorization", "Bearer access-token")
			if tc.body != "" {
				request.Header.Set("Content-Type", "application/json")
			}
			router.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusOK {
				t.Fatalf("expected admin AI prompt route, code=%d body=%s", recorder.Code, recorder.Body.String())
			}
		})
	}

	if promptService.listQuery.Keyword != "cat" || promptService.detailID != 9 || promptService.created.Status != aiprompt.StatusDisabled || promptService.updatedID != 7 || promptService.statusID != 7 || promptService.status != aiprompt.StatusDisabled || promptService.deletedID != 7 || !reflect.DeepEqual(promptService.batchDeletedIDs, []int64{3, 4}) {
		t.Fatalf("admin AI prompt routes not wired correctly: %#v", promptService)
	}
}

func TestRouterInstallsCanvasAIImageRoutesFromAiImageService(t *testing.T) {
	canvasService := &fakeRouterCanvasService{}
	canvasImageService := &fakeRouterAiImageService{}
	router := newTestRouter(t, testDependencies{
		Authenticator: func(ctx context.Context, input middleware.TokenInput) (*middleware.AuthIdentity, *apperror.Error) {
			return &middleware.AuthIdentity{UserID: 9, SessionID: 10, Platform: input.Platform}, nil
		},
		CanvasService:  canvasService,
		AiImageService: canvasImageService,
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/canvas/v1/ai/images?page=2&page_size=5&status=success", nil)
	request.Header.Set("Authorization", "Bearer canvas-token")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected AI image list status 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if canvasImageService.listUserID != 9 || canvasImageService.listQuery.Platform != enum.PlatformCanvas || canvasImageService.listQuery.CurrentPage != 2 || canvasImageService.listQuery.PageSize != 5 || canvasImageService.listQuery.Status != aiimage.StatusSuccess {
		t.Fatalf("expected AI image service List user=9 platform=canvas page=2 size=5 status=success, got user=%d query=%#v", canvasImageService.listUserID, canvasImageService.listQuery)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/canvas/v1/ai/images/generations", strings.NewReader(`{"agent_id":8,"prompt":"cat","n":2}`))
	request.Header.Set("Authorization", "Bearer canvas-token")
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected AI image generation status 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if canvasImageService.createInput.UserID != 9 || canvasImageService.createInput.Platform != enum.PlatformCanvas || canvasImageService.createInput.AgentID != 8 || canvasImageService.createInput.N != 2 {
		t.Fatalf("expected AI image service Create input from canvas route, got %#v", canvasImageService.createInput)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/canvas/v1/ai/images/88", nil)
	request.Header.Set("Authorization", "Bearer canvas-token")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected AI image detail status 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if canvasImageService.detailUserID != 9 || canvasImageService.detailTaskID != 88 || canvasImageService.detailPlatform != enum.PlatformCanvas {
		t.Fatalf("expected AI image service Detail user=9 task=88 platform=canvas, got user=%d task=%d platform=%q", canvasImageService.detailUserID, canvasImageService.detailTaskID, canvasImageService.detailPlatform)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodDelete, "/api/canvas/v1/ai/images/88", nil)
	request.Header.Set("Authorization", "Bearer canvas-token")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected AI image delete status 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if canvasImageService.deleteUserID != 9 || canvasImageService.deleteTaskID != 88 || canvasImageService.deletePlatform != enum.PlatformCanvas {
		t.Fatalf("expected AI image service Delete user=9 task=88 platform=canvas, got user=%d task=%d platform=%q", canvasImageService.deleteUserID, canvasImageService.deleteTaskID, canvasImageService.deletePlatform)
	}
}

func TestRouterInstallsCanvasAIChatRouteFromAIChatService(t *testing.T) {
	canvasService := &fakeRouterCanvasService{}
	aiChatService := &fakeRouterAIChatService{}
	router := newTestRouter(t, testDependencies{
		Authenticator: func(ctx context.Context, input middleware.TokenInput) (*middleware.AuthIdentity, *apperror.Error) {
			return &middleware.AuthIdentity{UserID: 9, SessionID: 10, Platform: input.Platform}, nil
		},
		CanvasService: canvasService,
		AiChatService: aiChatService,
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/canvas/v1/ai/chat/completions", strings.NewReader(`{"agent_id":8,"message":"hello"}`))
	request.Header.Set("Authorization", "Bearer canvas-token")
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected AI chat completion status 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if aiChatService.input.UserID != 9 || aiChatService.input.AgentID != 8 || aiChatService.input.Message != "hello" || aiChatService.input.ModelID != "" {
		t.Fatalf("expected AI chat service input from canvas route, got %#v", aiChatService.input)
	}
}

func TestRouterInstallsCanvasAIVideoRoutesFromAIVideoService(t *testing.T) {
	canvasService := &fakeRouterCanvasService{}
	aiVideoService := &fakeRouterAIVideoService{}
	router := newTestRouter(t, testDependencies{
		Authenticator: func(ctx context.Context, input middleware.TokenInput) (*middleware.AuthIdentity, *apperror.Error) {
			return &middleware.AuthIdentity{UserID: 9, SessionID: 10, Platform: input.Platform}, nil
		},
		CanvasService:  canvasService,
		AiVideoService: aiVideoService,
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/canvas/v1/ai/videos", strings.NewReader(`{"agent_id":8,"prompt":"clip","duration_seconds":4,"size":"1280x720","resolution_name":"720p"}`))
	request.Header.Set("Authorization", "Bearer canvas-token")
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected AI video create status 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if aiVideoService.createInput.UserID != 9 || aiVideoService.createInput.AgentID != 8 || aiVideoService.createInput.Prompt != "clip" || aiVideoService.createInput.DurationSeconds != 4 || aiVideoService.createInput.Size != "1280x720" || aiVideoService.createInput.ResolutionName != "720p" || aiVideoService.createInput.ModelID != "" {
		t.Fatalf("expected AI video service Create input from canvas route, got %#v", aiVideoService.createInput)
	}

	referenceBody := &bytes.Buffer{}
	writer := multipart.NewWriter(referenceBody)
	if err := writer.WriteField("media_kind", "video"); err != nil {
		t.Fatalf("write reference media kind: %v", err)
	}
	part, err := writer.CreateFormFile("file", "reference.mp4")
	if err != nil {
		t.Fatalf("create reference media file: %v", err)
	}
	if _, err := part.Write([]byte("reference-video")); err != nil {
		t.Fatalf("write reference media file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close reference media multipart: %v", err)
	}
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/canvas/v1/ai/videos/reference-media", referenceBody)
	request.Header.Set("Authorization", "Bearer canvas-token")
	request.Header.Set("Content-Type", writer.FormDataContentType())
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected AI video reference media upload status 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if aiVideoService.referenceUploadInput.UserID != 9 || aiVideoService.referenceUploadInput.MediaKind != "video" || aiVideoService.referenceUploadInput.MimeType != "video/mp4" || string(aiVideoService.referenceUploadInput.Body) != "reference-video" {
		t.Fatalf("expected AI video reference upload input from canvas route, got %#v", aiVideoService.referenceUploadInput)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/canvas/v1/ai/videos/99", nil)
	request.Header.Set("Authorization", "Bearer canvas-token")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected AI video status 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if aiVideoService.statusUserID != 9 || aiVideoService.statusID != 99 {
		t.Fatalf("expected AI video service Status user=9 id=99, got user=%d id=%d", aiVideoService.statusUserID, aiVideoService.statusID)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/canvas/v1/ai/videos/99/content", nil)
	request.Header.Set("Authorization", "Bearer canvas-token")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || recorder.Body.String() != "video" || recorder.Header().Get("Content-Type") != "video/mp4" {
		t.Fatalf("expected AI video content route, got code=%d type=%s body=%s", recorder.Code, recorder.Header().Get("Content-Type"), recorder.Body.String())
	}
	if aiVideoService.contentUserID != 9 || aiVideoService.contentID != 99 {
		t.Fatalf("expected AI video service Content user=9 id=99, got user=%d id=%d", aiVideoService.contentUserID, aiVideoService.contentID)
	}
}

func TestRouterInstallsCanvasAIAudioRouteFromAIAudioService(t *testing.T) {
	canvasService := &fakeRouterCanvasService{}
	aiAudioService := &fakeRouterAIAudioService{}
	router := newTestRouter(t, testDependencies{
		Authenticator: func(ctx context.Context, input middleware.TokenInput) (*middleware.AuthIdentity, *apperror.Error) {
			return &middleware.AuthIdentity{UserID: 9, SessionID: 10, Platform: input.Platform}, nil
		},
		CanvasService:  canvasService,
		AiAudioService: aiAudioService,
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/canvas/v1/ai/audios", strings.NewReader(`{"agent_id":8,"prompt":"voice over","voice":"nova","response_format":"mp3","speed":1.1,"instructions":"warm"}`))
	request.Header.Set("Authorization", "Bearer canvas-token")
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK || recorder.Body.String() != "audio" || recorder.Header().Get("Content-Type") != "audio/mpeg" {
		t.Fatalf("expected AI audio route, got code=%d type=%s body=%s", recorder.Code, recorder.Header().Get("Content-Type"), recorder.Body.String())
	}
	if aiAudioService.input.UserID != 9 || aiAudioService.input.AgentID != 8 || aiAudioService.input.Prompt != "voice over" || aiAudioService.input.ModelID != "" || aiAudioService.input.Voice != "nova" || aiAudioService.input.ResponseFormat != "mp3" || aiAudioService.input.Speed == nil || *aiAudioService.input.Speed != 1.1 || aiAudioService.input.Instructions != "warm" {
		t.Fatalf("expected AI audio service input from canvas route, got %#v", aiAudioService.input)
	}
}

func TestRouterInstallsCanvasWalletAndRechargeRoutes(t *testing.T) {
	paymentService := &fakeRouterPaymentService{}
	walletService := &fakeRouterWalletService{}
	router := newRouterFromTestDependencies(testDependencies{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Authenticator: func(ctx context.Context, input middleware.TokenInput) (*middleware.AuthIdentity, *apperror.Error) {
			return &middleware.AuthIdentity{UserID: 9, SessionID: 10, Platform: input.Platform}, nil
		},
		PaymentService: paymentService,
		WalletService:  walletService,
	})

	routes := map[string]bool{}
	for _, route := range router.Routes() {
		routes[route.Method+" "+route.Path] = true
	}
	for _, route := range []string{
		http.MethodGet + " /api/canvas/v1/auth/login-config",
		http.MethodGet + " /api/canvas/v1/auth/captcha",
		http.MethodPost + " /api/canvas/v1/auth/send-code",
		http.MethodPost + " /api/canvas/v1/auth/login",
		http.MethodPost + " /api/canvas/v1/auth/logout",
		http.MethodGet + " /api/canvas/v1/users/me",
		http.MethodGet + " /api/canvas/v1/profile",
		http.MethodPut + " /api/canvas/v1/profile",
		http.MethodGet + " /api/canvas/v1/wallet/summary",
		http.MethodGet + " /api/canvas/v1/wallet/transactions",
		http.MethodGet + " /api/canvas/v1/payment/recharges/page-init",
		http.MethodGet + " /api/canvas/v1/payment/recharges",
		http.MethodPost + " /api/canvas/v1/payment/recharges",
		http.MethodPost + " /api/canvas/v1/payment/recharges/:id/pay",
	} {
		if !routes[route] {
			t.Fatalf("expected canvas route to be installed: %s", route)
		}
	}
	for _, route := range []string{
		http.MethodGet + " /api/canvas/v1/payment/ledger",
		http.MethodGet + " /api/canvas/v1/payment/wallets",
		http.MethodPost + " /api/canvas/v1/wallet/consumptions",
		http.MethodPost + " /api/canvas/v1/payment/recharges/:id/sync",
		http.MethodPatch + " /api/canvas/v1/payment/recharges/:id/close",
	} {
		if routes[route] {
			t.Fatalf("canvas route must not be installed: %s", route)
		}
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/canvas/v1/wallet/summary", nil)
	request.Header.Set("Authorization", "Bearer canvas-token")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || walletService.summaryUserID != 9 {
		t.Fatalf("expected canvas wallet summary for token user, code=%d body=%s user=%d", recorder.Code, recorder.Body.String(), walletService.summaryUserID)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/canvas/v1/wallet/transactions?current_page=2&page_size=10&user_id=999&keyword=WLT", nil)
	request.Header.Set("Authorization", "Bearer canvas-token")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || walletService.query.UserID != 9 || walletService.query.CurrentPage != 2 || walletService.query.Keyword != "WLT" {
		t.Fatalf("expected canvas wallet transactions to force token user, code=%d body=%s query=%#v", recorder.Code, recorder.Body.String(), walletService.query)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/canvas/v1/payment/recharges/page-init", nil)
	request.Header.Set("Authorization", "Bearer canvas-token")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected canvas recharge page-init, code=%d body=%s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/canvas/v1/payment/recharges?current_page=1&page_size=10&status=paying", nil)
	request.Header.Set("Authorization", "Bearer canvas-token")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || paymentService.rechargeQuery.UserID != 9 || paymentService.rechargeQuery.Status != "paying" {
		t.Fatalf("expected canvas recharge list to force token user, code=%d body=%s query=%#v", recorder.Code, recorder.Body.String(), paymentService.rechargeQuery)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/canvas/v1/payment/recharges", strings.NewReader(`{"package_code":"recharge_10","pay_method":"web","return_url":"https://canvas.example.test/recharge"}`))
	request.Header.Set("Authorization", "Bearer canvas-token")
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || paymentService.rechargeInput.UserID != 9 || paymentService.rechargeInput.PackageCode != "recharge_10" {
		t.Fatalf("expected canvas recharge create to force token user, code=%d body=%s input=%#v", recorder.Code, recorder.Body.String(), paymentService.rechargeInput)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/canvas/v1/payment/recharges/2/pay", nil)
	request.Header.Set("Authorization", "Bearer canvas-token")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || paymentService.payID != 2 {
		t.Fatalf("expected canvas recharge pay, code=%d body=%s service=%#v", recorder.Code, recorder.Body.String(), paymentService)
	}

	for _, retired := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/canvas/v1/payment/ledger"},
		{http.MethodGet, "/api/canvas/v1/payment/wallets"},
		{http.MethodPost, "/api/canvas/v1/wallet/consumptions"},
		{http.MethodPost, "/api/canvas/v1/payment/recharges/2/sync"},
		{http.MethodPatch, "/api/canvas/v1/payment/recharges/2/close"},
	} {
		recorder = httptest.NewRecorder()
		request = httptest.NewRequest(retired.method, retired.path, nil)
		request.Header.Set("Authorization", "Bearer canvas-token")
		router.ServeHTTP(recorder, request)
		if recorder.Code == http.StatusOK {
			t.Fatalf("canvas must not expose route: %s %s", retired.method, retired.path)
		}
	}
}
func TestRouterInstallsSystemLogReadOnlyRESTRoutes(t *testing.T) {
	systemLogService := &fakeRouterSystemLogService{}
	router := newTestRouter(t, testDependencies{
		Authenticator: func(ctx context.Context, input middleware.TokenInput) (*middleware.AuthIdentity, *apperror.Error) {
			return &middleware.AuthIdentity{UserID: 1, SessionID: 10, Platform: "admin"}, nil
		},
		SystemLogService: systemLogService,
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/system-logs/files", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected system log files status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if !systemLogService.filesCalled {
		t.Fatalf("expected system log files service call")
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/admin/v1/system-logs/files/admin-api.log/lines?tail=500&level=ERROR&keyword=boom", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected system log lines status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if systemLogService.linesQuery.Filename != "admin-api.log" || systemLogService.linesQuery.Tail != 500 || systemLogService.linesQuery.Level != "ERROR" || systemLogService.linesQuery.Keyword != "boom" {
		t.Fatalf("system log lines query mismatch: %#v", systemLogService.linesQuery)
	}
}

func TestRouterInstallsUploadTokenCreateRoute(t *testing.T) {
	uploadTokenService := &fakeRouterUploadTokenService{}
	permissionChecked := false
	router := newTestRouter(t, testDependencies{
		Authenticator: func(ctx context.Context, input middleware.TokenInput) (*middleware.AuthIdentity, *apperror.Error) {
			return &middleware.AuthIdentity{UserID: 1, SessionID: 10, Platform: "admin"}, nil
		},
		PermissionRules: map[middleware.RouteKey]string{},
		PermissionChecker: func(ctx context.Context, input middleware.PermissionInput) *apperror.Error {
			permissionChecked = true
			return nil
		},
		UploadTokenService: uploadTokenService,
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/v1/upload-tokens", strings.NewReader(`{"folder":"ai-agents","file_name":"77ebbddc-e755-441f-856b-09a9c4f2bfff.jpg","file_size":133106,"file_kind":"image"}`))
	request.Header.Set("Authorization", "Bearer access-token")
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected upload token status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if permissionChecked {
		t.Fatalf("upload token create must only require login and must not run RBAC permission checker")
	}
	if uploadTokenService.input.Folder != "ai-agents" || uploadTokenService.input.FileName != "77ebbddc-e755-441f-856b-09a9c4f2bfff.jpg" || uploadTokenService.input.FileSize != 133106 || uploadTokenService.input.FileKind != "image" {
		t.Fatalf("upload token input mismatch: %#v", uploadTokenService.input)
	}
	body := decodeRouterBody(t, recorder)
	data := mustRouterData(t, body)
	if data["provider"] != "cos" {
		t.Fatalf("expected cos provider, got %#v", data["provider"])
	}
}

func TestRouterInstallsQueueMonitorReadOnlyRESTRoutes(t *testing.T) {
	queueMonitorService := &fakeRouterQueueMonitorService{}
	queueMonitorUI := &fakeQueueMonitorUI{}
	browserGrants := newRouterBrowserGrants()
	queueGrant, grantErr := browserGrants.IssueQueueMonitorGrant(context.Background(), auth.GrantSubject{UserID: 1, SessionID: 10, Platform: "admin"})
	if grantErr != nil {
		t.Fatalf("issue queue monitor grant: %v", grantErr)
	}
	router := newTestRouter(t, testDependencies{
		Authenticator: func(ctx context.Context, input middleware.TokenInput) (*middleware.AuthIdentity, *apperror.Error) {
			return &middleware.AuthIdentity{UserID: 1, SessionID: 10, Platform: "admin"}, nil
		},
		QueueMonitorService: queueMonitorService,
		QueueMonitorUI:      queueMonitorUI,
		BrowserGrants:       browserGrants,
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/queue-monitor", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if !queueMonitorService.listCalled {
		t.Fatalf("expected queue monitor list call")
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/admin/v1/queue-monitor/failed?queue=critical&current_page=2&page_size=10", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if queueMonitorService.failedListQuery.Queue != "critical" || queueMonitorService.failedListQuery.CurrentPage != 2 || queueMonitorService.failedListQuery.PageSize != 10 {
		t.Fatalf("queue monitor failed query mismatch: %#v", queueMonitorService.failedListQuery)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, queuemonitor.UIPath+"/api/queues", nil)
	request.AddCookie(&http.Cookie{Name: authadmin.QueueMonitorGrantCookieName, Value: queueGrant.Credential})
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected queue monitor UI status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if !queueMonitorUI.called || queueMonitorUI.path != queuemonitor.UIPath+"/api/queues" || queueMonitorUI.method != http.MethodGet {
		t.Fatalf("queue monitor UI handler not called as expected: %#v", queueMonitorUI)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, queuemonitor.UIPath, nil)
	request.AddCookie(&http.Cookie{Name: authadmin.QueueMonitorGrantCookieName, Value: queueGrant.Credential})
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected queue monitor UI cookie status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
}

func TestRouterInstallsPermissionCheckAfterAuthToken(t *testing.T) {
	router := newTestRouter(t, testDependencies{
		Authenticator: func(ctx context.Context, input middleware.TokenInput) (*middleware.AuthIdentity, *apperror.Error) {
			return &middleware.AuthIdentity{UserID: 1, SessionID: 10, Platform: "admin"}, nil
		},
		PermissionRules: map[middleware.RouteKey]string{
			middleware.NewRouteKey(http.MethodGet, "/api/admin/v1/users/me"): "user:me",
		},
		PermissionChecker: func(ctx context.Context, input middleware.PermissionInput) *apperror.Error {
			if input.UserID != 1 || input.Code != "user:me" {
				t.Fatalf("unexpected permission input: %#v", input)
			}
			return apperror.Forbidden("无接口权限")
		},
		UserService: &fakeRouterUserService{},
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/users/me", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusForbidden, recorder.Code, recorder.Body.String())
	}
	body := decodeRouterBody(t, recorder)
	if body["msg"] != "无接口权限" {
		t.Fatalf("expected permission denial, got %#v", body)
	}
}

func TestRouterInstallsOperationLogAfterPermissionCheck(t *testing.T) {
	var got middleware.OperationInput
	router := newTestRouter(t, testDependencies{
		Authenticator: func(ctx context.Context, input middleware.TokenInput) (*middleware.AuthIdentity, *apperror.Error) {
			return &middleware.AuthIdentity{UserID: 1, SessionID: 10, Platform: "admin"}, nil
		},
		OperationRules: map[middleware.RouteKey]middleware.OperationRule{
			middleware.NewRouteKey(http.MethodGet, "/api/admin/v1/users/me"): {Module: "user", Action: "me", Title: "查看当前用户"},
		},
		OperationRecorder: func(ctx context.Context, input middleware.OperationInput) error {
			got = input
			return nil
		},
		UserService: &fakeRouterUserService{result: &user.InitResponse{UserID: 1, Username: "admin"}},
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/users/me", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if got.UserID != 1 || got.Module != "user" || got.Action != "me" || got.Status != http.StatusOK || !got.Success {
		t.Fatalf("unexpected operation input: %#v", got)
	}
}

func TestRealtimeRouteRequiresAuthAndUpgradesWebSocket(t *testing.T) {
	browserGrants := newRouterBrowserGrants()
	ticket, ticketErr := browserGrants.IssueRealtimeTicket(context.Background(), auth.GrantSubject{UserID: 7, SessionID: 9, Platform: "admin"})
	if ticketErr != nil {
		t.Fatalf("issue realtime ticket: %v", ticketErr)
	}
	router := newTestRouter(t, testDependencies{
		BrowserGrants: browserGrants,
		RealtimeHandler: realtimeadmin.NewHandler(
			realtimemodule.NewService(25*time.Second),
			infrarealtime.NewUpgrader(func(*http.Request) bool { return true }),
			infrarealtime.NewManager(),
			slog.New(slog.NewTextHandler(io.Discard, nil)),
		),
	})
	server := httptest.NewServer(router)
	defer server.Close()

	client, _, err := websocket.DefaultDialer.Dial("ws"+server.URL[len("http"):]+realtimeadmin.WSPath+"?ticket="+ticket.Credential, nil)
	if err != nil {
		t.Fatalf("dial realtime: %v", err)
	}
	defer func() { _ = client.Close() }()

	var connected map[string]any
	if err := client.ReadJSON(&connected); err != nil {
		t.Fatalf("read connected: %v", err)
	}
	if connected["type"] != realtimemodule.TypeConnectedV1 {
		t.Fatalf("expected connected event, got %#v", connected)
	}
}

func TestRealtimeRouteRejectsLegacyCookieTokenForBrowserWebSocket(t *testing.T) {
	router := newTestRouter(t, testDependencies{
		RealtimeHandler: realtimeadmin.NewHandler(
			realtimemodule.NewService(25*time.Second),
			infrarealtime.NewUpgrader(func(*http.Request) bool { return true }),
			infrarealtime.NewManager(),
			slog.New(slog.NewTextHandler(io.Discard, nil)),
		),
	})
	server := httptest.NewServer(router)
	defer server.Close()

	client, _, err := websocket.DefaultDialer.Dial("ws"+server.URL[len("http"):]+realtimeadmin.WSPath, http.Header{
		"Cookie": []string{"access_token=cookie-access-token"},
	})
	if err == nil {
		_ = client.Close()
		t.Fatal("legacy access cookie unexpectedly authenticated websocket")
	}
}

func TestRealtimeRouteAllowsConfiguredBrowserOrigin(t *testing.T) {
	browserGrants := newRouterBrowserGrants()
	ticket, ticketErr := browserGrants.IssueRealtimeTicket(context.Background(), auth.GrantSubject{UserID: 7, SessionID: 9, Platform: "admin"})
	if ticketErr != nil {
		t.Fatalf("issue realtime ticket: %v", ticketErr)
	}
	router := newTestRouter(t, testDependencies{
		CORS: config.CORSConfig{
			AllowOrigins:     []string{"http://127.0.0.1:5173"},
			AllowMethods:     []string{"GET", "OPTIONS"},
			AllowHeaders:     []string{"Authorization", "platform", "device-id"},
			AllowCredentials: true,
		},
		BrowserGrants: browserGrants,
		RealtimeHandler: realtimeadmin.NewHandler(
			realtimemodule.NewService(25*time.Second),
			infrarealtime.NewUpgrader(infrarealtime.NewAllowedOriginChecker([]string{"http://127.0.0.1:5173"})),
			infrarealtime.NewManager(),
			slog.New(slog.NewTextHandler(io.Discard, nil)),
		),
	})
	server := httptest.NewServer(router)
	defer server.Close()

	client, _, err := websocket.DefaultDialer.Dial("ws"+server.URL[len("http"):]+realtimeadmin.WSPath+"?ticket="+ticket.Credential, http.Header{
		"Origin": []string{"http://127.0.0.1:5173"},
	})
	if err != nil {
		t.Fatalf("dial realtime from configured origin: %v", err)
	}
	defer func() { _ = client.Close() }()

	var connected map[string]any
	if err := client.ReadJSON(&connected); err != nil {
		t.Fatalf("read connected: %v", err)
	}
	if connected["type"] != realtimemodule.TypeConnectedV1 {
		t.Fatalf("expected connected event, got %#v", connected)
	}
}

func TestRouterInstallsAIKnowledgeRESTRoutes(t *testing.T) {
	knowledgeService := &fakeRouterAIKnowledgeService{}
	router := newTestRouter(t, testDependencies{
		Authenticator: func(ctx context.Context, input middleware.TokenInput) (*middleware.AuthIdentity, *apperror.Error) {
			return &middleware.AuthIdentity{UserID: 7, SessionID: 9, Platform: "admin"}, nil
		},
		AiKnowledgeService: knowledgeService,
	})

	cases := []struct{ method, path, body string }{
		{http.MethodGet, "/api/admin/v1/ai-knowledge-bases/page-init", ""},
		{http.MethodGet, "/api/admin/v1/ai-knowledge-bases?current_page=1&page_size=20&code=arch&status=1", ""},
		{http.MethodPost, "/api/admin/v1/ai-knowledge-bases", `{"name":"架构库","code":"arch","description":"docs","chunk_size_chars":1200,"chunk_overlap_chars":120,"default_top_k":5,"default_min_score":0.1,"default_max_context_chars":6000,"status":1}`},
		{http.MethodGet, "/api/admin/v1/ai-knowledge-bases/1", ""},
		{http.MethodPut, "/api/admin/v1/ai-knowledge-bases/1", `{"name":"架构库","code":"arch","description":"docs","chunk_size_chars":1200,"chunk_overlap_chars":120,"default_top_k":5,"default_min_score":0.1,"default_max_context_chars":6000,"status":1}`},
		{http.MethodPatch, "/api/admin/v1/ai-knowledge-bases/1/status", `{"status":1}`},
		{http.MethodDelete, "/api/admin/v1/ai-knowledge-bases/1", ""},
		{http.MethodGet, "/api/admin/v1/ai-knowledge-bases/1/documents", ""},
		{http.MethodPost, "/api/admin/v1/ai-knowledge-bases/1/documents", `{"title":"FAQ","source_type":"text","content":"hello","status":1}`},
		{http.MethodGet, "/api/admin/v1/ai-knowledge-documents/2", ""},
		{http.MethodPut, "/api/admin/v1/ai-knowledge-documents/2", `{"title":"FAQ","source_type":"text","content":"hello","status":1}`},
		{http.MethodPatch, "/api/admin/v1/ai-knowledge-documents/2/status", `{"status":1}`},
		{http.MethodDelete, "/api/admin/v1/ai-knowledge-documents/2", ""},
		{http.MethodPost, "/api/admin/v1/ai-knowledge-documents/2/reindex", ""},
		{http.MethodGet, "/api/admin/v1/ai-knowledge-documents/2/chunks", ""},
		{http.MethodPost, "/api/admin/v1/ai-knowledge-bases/1/retrieval-tests", `{"query":"Gin modular monolith"}`},
		{http.MethodGet, "/api/admin/v1/ai-agents/7/knowledge-bases", ""},
		{http.MethodPut, "/api/admin/v1/ai-agents/7/knowledge-bases", `{"bindings":[{"knowledge_base_id":1,"top_k":5,"min_score":0.1,"max_context_chars":6000,"status":1}]}`},
	}
	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			var body io.Reader
			if tc.body != "" {
				body = strings.NewReader(tc.body)
			}
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(tc.method, tc.path, body)
			request.Header.Set("Authorization", "Bearer access-token")
			if tc.body != "" {
				request.Header.Set("Content-Type", "application/json")
			}
			router.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusOK {
				t.Fatalf("expected status 200, got %d body=%s", recorder.Code, recorder.Body.String())
			}
		})
	}
	if !knowledgeService.initCalled || knowledgeService.listQuery.Code != "arch" || knowledgeService.listQuery.Status == nil || *knowledgeService.listQuery.Status != enum.CommonYes {
		t.Fatalf("AI knowledge init/list not routed correctly: called=%v query=%#v", knowledgeService.initCalled, knowledgeService.listQuery)
	}
	if knowledgeService.detailID != 1 || knowledgeService.documentsBaseID != 1 || knowledgeService.createdDocumentBaseID != 1 || knowledgeService.documentDetailID != 2 || knowledgeService.documentUpdateID != 2 || knowledgeService.documentStatusID != 2 || knowledgeService.deletedDocumentID != 2 || knowledgeService.reindexDocumentID != 2 || knowledgeService.chunksDocumentID != 2 || knowledgeService.retrievalTestBaseID != 1 || knowledgeService.agentBindingsID != 7 || knowledgeService.updatedAgentBindingsID != 7 {
		t.Fatalf("AI knowledge nested routes not called correctly: %#v", knowledgeService)
	}
}

func TestRouterDoesNotInstallRetiredAIRoutes(t *testing.T) {
	router := newTestRouter(t, testDependencies{
		Authenticator: func(ctx context.Context, input middleware.TokenInput) (*middleware.AuthIdentity, *apperror.Error) {
			return &middleware.AuthIdentity{UserID: 7, SessionID: 9, Platform: "admin"}, nil
		},
		AiProviderService:  &fakeRouterAIProviderService{},
		AiAgentService:     &fakeRouterAIAgentService{},
		AiKnowledgeService: &fakeRouterAIKnowledgeService{},
		AiToolService:      &fakeRouterAIToolService{},
	})

	retired := []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodGet, "/api/admin/v1/ai-models/page-init", ""},
		{http.MethodGet, "/api/admin/v1/ai-models", ""},
		{http.MethodPost, "/api/admin/v1/ai-models", `{"name":"model"}`},
		{http.MethodGet, "/api/admin/v1/ai-knowledge-maps/page-init", ""},
		{http.MethodGet, "/api/admin/v1/ai-knowledge-maps", ""},
		{http.MethodPost, "/api/admin/v1/ai-knowledge-maps", `{"name":"kb"}`},
	}
	for _, tc := range retired {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			var body io.Reader
			if tc.body != "" {
				body = strings.NewReader(tc.body)
			}
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(tc.method, tc.path, body)
			request.Header.Set("Authorization", "Bearer access-token")
			if tc.body != "" {
				request.Header.Set("Content-Type", "application/json")
			}
			router.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusNotFound {
				t.Fatalf("retired AI route must not be installed, got status=%d body=%s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestRouterLocalizesAuthTokenErrors(t *testing.T) {
	router := newRouterFromTestDependencies(testDependencies{})

	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/users/me", nil)
	request.Header.Set("Accept-Language", "en-US")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["msg"] != "Missing token" {
		t.Fatalf("expected localized missing token, got %#v body=%s", body["msg"], recorder.Body.String())
	}
}

func TestRouterPassesTelemetryToAccessLogUsingRegisteredRoute(t *testing.T) {
	recorder := telemetry.NewMemoryRecorder()
	router := newTestRouter(t, testDependencies{Telemetry: recorder})
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/health?token=private", nil))

	events := recorder.Events()
	if len(events) != 2 {
		t.Fatalf("expected router HTTP telemetry, got %+v", events)
	}
	for _, event := range events {
		if event.Attributes["http.route"] != "/health" || event.Attributes["http.method"] != http.MethodGet {
			t.Fatalf("router telemetry mismatch: %+v", event)
		}
	}
}

func TestRouterInstallsAIRuntimeRESTRoutes(t *testing.T) {
	router := newTestRouter(t, testDependencies{
		Authenticator: func(ctx context.Context, input middleware.TokenInput) (*middleware.AuthIdentity, *apperror.Error) {
			return &middleware.AuthIdentity{UserID: 7, SessionID: 9, Platform: "admin"}, nil
		},
		AiConversationService: fakeRouterAIConversationService{},
		AiMessageService:      fakeRouterAIMessageService{},
		AiRunService:          fakeRouterAIRunService{},
		AiChatService:         &fakeRouterAIChatService{},
	})

	cases := []struct{ method, path, body string }{
		{http.MethodGet, "/api/admin/v1/ai-conversations", ""},
		{http.MethodGet, "/api/admin/v1/ai-conversations/1", ""},
		{http.MethodPost, "/api/admin/v1/ai-conversations", `{"agent_id":1,"title":"会话"}`},
		{http.MethodPut, "/api/admin/v1/ai-conversations/1", `{"title":"新会话"}`},
		{http.MethodDelete, "/api/admin/v1/ai-conversations/1", ""},
		{http.MethodGet, "/api/admin/v1/ai-conversations/1/messages", ""},
		{http.MethodPost, "/api/admin/v1/ai-conversations/1/messages", `{"content":"hello","request_id":"rid"}`},
		{http.MethodPost, "/api/admin/v1/ai-conversations/1/messages/cancel", `{"request_id":"rid"}`},
		{http.MethodGet, "/api/admin/v1/ai-runs/page-init", ""},
		{http.MethodGet, "/api/admin/v1/ai-runs", ""},
		{http.MethodGet, "/api/admin/v1/ai-runs/1", ""},
		{http.MethodGet, "/api/admin/v1/ai-runs/stats", ""},
		{http.MethodGet, "/api/admin/v1/ai-runs/stats/by-date", ""},
		{http.MethodGet, "/api/admin/v1/ai-runs/stats/by-agent", ""},
		{http.MethodGet, "/api/admin/v1/ai-runs/stats/by-user", ""},
	}
	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			var body io.Reader
			if tc.body != "" {
				body = strings.NewReader(tc.body)
			}
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(tc.method, tc.path, body)
			request.Header.Set("Authorization", "Bearer access-token")
			if tc.body != "" {
				request.Header.Set("Content-Type", "application/json")
			}
			router.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusOK {
				t.Fatalf("expected status 200, got %d body=%s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestAdminRouteSnapshot(t *testing.T) {
	router := newRouterFromTestDependencies(testDependencies{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})

	var routes []string
	for _, route := range router.Routes() {
		path := route.Path
		if strings.HasPrefix(path, "/api/admin/v1/") ||
			path == "/api/app/v1/users/me" ||
			path == "/api/canvas/v1/users/me" ||
			strings.HasPrefix(path, "/api/payment/callbacks/") ||
			path == "/health" || path == "/ready" {
			routes = append(routes, route.Method+" "+path)
		}
	}
	sort.Strings(routes)

	goldenPath := filepath.Join("testdata", "admin_routes_golden.txt")
	if os.Getenv("UPDATE_ADMIN_ROUTE_SNAPSHOT") == "1" {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
			t.Fatalf("create testdata: %v", err)
		}
		if err := os.WriteFile(goldenPath, []byte(strings.Join(routes, "\n")+"\n"), 0o644); err != nil {
			t.Fatalf("write route snapshot: %v", err)
		}
	}

	wantBytes, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read route snapshot: %v", err)
	}
	want := strings.TrimSpace(strings.ReplaceAll(string(wantBytes), "\r\n", "\n"))
	got := strings.Join(routes, "\n")
	if got != want {
		t.Fatalf("admin route snapshot mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func newTestRouter(t *testing.T, deps testDependencies) http.Handler {
	t.Helper()
	if deps.Logger == nil {
		deps.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return newRouterFromTestDependencies(deps)
}

func decodeRouterBody(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid json response: %v", err)
	}
	return body
}

func decodeRouterLogEntry(t *testing.T, data []byte) map[string]any {
	t.Helper()
	var entry map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(data), &entry); err != nil {
		t.Fatalf("invalid json log entry: %v\n%s", err, data)
	}
	return entry
}

func mustRouterData(t *testing.T, body map[string]any) map[string]any {
	t.Helper()
	data, ok := body["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected data object, got %#v", body["data"])
	}
	return data
}

func routerStringSliceEqual(value any, want []string) bool {
	got, ok := value.([]any)
	if !ok || len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func routerRouteSliceContains(value any, wantPath string) bool {
	got, ok := value.([]any)
	if !ok {
		return false
	}
	for _, raw := range got {
		item, ok := raw.(map[string]any)
		if ok && item["path"] == wantPath {
			return true
		}
	}
	return false
}

func assertRequestID(t *testing.T, recorder *httptest.ResponseRecorder) {
	t.Helper()
	if recorder.Header().Get("X-Request-Id") == "" {
		t.Fatalf("expected X-Request-Id header")
	}
}
