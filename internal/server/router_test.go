package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/config"
	"admin_back_go/internal/enum"
	"admin_back_go/internal/middleware"
	"admin_back_go/internal/module/aiagent"
	"admin_back_go/internal/module/aiconversation"
	"admin_back_go/internal/module/aiknowledge"
	"admin_back_go/internal/module/aimessage"
	"admin_back_go/internal/module/aiprovider"
	"admin_back_go/internal/module/airun"
	"admin_back_go/internal/module/aitool"
	"admin_back_go/internal/module/auth"
	"admin_back_go/internal/module/authplatform"
	"admin_back_go/internal/module/captcha"
	"admin_back_go/internal/module/clientversion"
	"admin_back_go/internal/module/crontask"
	"admin_back_go/internal/module/exporttask"
	"admin_back_go/internal/module/notification"
	"admin_back_go/internal/module/notificationtask"
	"admin_back_go/internal/module/operationlog"
	"admin_back_go/internal/module/payment"
	"admin_back_go/internal/module/permission"
	"admin_back_go/internal/module/queuemonitor"
	realtimemodule "admin_back_go/internal/module/realtime"
	"admin_back_go/internal/module/role"
	"admin_back_go/internal/module/session"
	"admin_back_go/internal/module/systemlog"
	"admin_back_go/internal/module/systemsetting"
	"admin_back_go/internal/module/uploadtoken"
	"admin_back_go/internal/module/user"
	"admin_back_go/internal/module/userloginlog"
	"admin_back_go/internal/module/userquickentry"
	"admin_back_go/internal/module/usersession"
	platformai "admin_back_go/internal/platform/ai"
	platformrealtime "admin_back_go/internal/platform/realtime"
	"admin_back_go/internal/readiness"

	"github.com/gorilla/websocket"
)

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

func (f *fakeRouterAIKnowledgeService) Init(ctx context.Context) (*aiknowledge.InitResponse, *apperror.Error) {
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

type fakeRouterAIRunService struct{}

func (fakeRouterAIRunService) Init(ctx context.Context) (*airun.InitResponse, *apperror.Error) {
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

type fakeRouterAIChatService struct{}

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
	return "验证码发送成功(测试:123456)", nil
}

func (fakeAuthService) LoginConfig(ctx context.Context, platform string) (*auth.LoginConfigResponse, *apperror.Error) {
	return &auth.LoginConfigResponse{
		LoginTypeArr:   []auth.LoginTypeOption{{Label: "密码登录", Value: auth.LoginTypePassword}},
		CaptchaEnabled: true,
		CaptchaType:    captcha.TypeSlide,
	}, nil
}

func (fakeAuthService) Refresh(ctx context.Context, input session.RefreshInput) (*session.TokenResult, *apperror.Error) {
	return &session.TokenResult{
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

func (fakeCaptchaService) Generate(ctx context.Context) (*captcha.ChallengeResponse, *apperror.Error) {
	return &captcha.ChallengeResponse{
		CaptchaID:   "captcha-id",
		CaptchaType: captcha.TypeSlide,
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
	listQuery      user.ListQuery
	listResult     *user.ListResponse
	exportInput    user.ExportInput
}

type fakeRouterUserSessionService struct {
	listQuery      usersession.ListQuery
	revokeID       int64
	batchInput     usersession.BatchRevokeInput
	currentSession int64
}

func (fakeRouterUserSessionService) PageInit(ctx context.Context) (*usersession.PageInitResponse, *apperror.Error) {
	return &usersession.PageInitResponse{}, nil
}

func (f *fakeRouterUserSessionService) List(ctx context.Context, query usersession.ListQuery) (*usersession.ListResponse, *apperror.Error) {
	f.listQuery = query
	return &usersession.ListResponse{
		List: []usersession.ListItem{{ID: 1, UserID: 2, Username: "admin", Platform: "admin", Status: usersession.SessionStatusActive}},
		Page: usersession.Page{CurrentPage: query.CurrentPage, PageSize: query.PageSize, Total: 1, TotalPage: 1},
	}, nil
}

func (fakeRouterUserSessionService) Stats(ctx context.Context) (*usersession.StatsResponse, *apperror.Error) {
	return &usersession.StatsResponse{TotalActive: 0, PlatformDistribution: map[string]int64{"admin": 0, "app": 0}}, nil
}

func (f *fakeRouterUserSessionService) Revoke(ctx context.Context, id int64, currentSessionID int64) (*usersession.RevokeResponse, *apperror.Error) {
	f.revokeID = id
	f.currentSession = currentSessionID
	return &usersession.RevokeResponse{ID: id, Revoked: true}, nil
}

func (f *fakeRouterUserSessionService) BatchRevoke(ctx context.Context, input usersession.BatchRevokeInput, currentSessionID int64) (*usersession.BatchRevokeResponse, *apperror.Error) {
	f.batchInput = input
	f.currentSession = currentSessionID
	return &usersession.BatchRevokeResponse{Count: int64(len(input.IDs))}, nil
}

type fakeRouterUserQuickEntryService struct {
	userID int64
	input  userquickentry.SaveInput
}

func (f *fakeRouterUserQuickEntryService) Save(ctx context.Context, userID int64, input userquickentry.SaveInput) (*userquickentry.SaveResponse, *apperror.Error) {
	f.userID = userID
	f.input = input
	return &userquickentry.SaveResponse{QuickEntry: []userquickentry.QuickEntry{{ID: 1, PermissionID: 2, Sort: 1}}}, nil
}

type fakeRouterUserLoginLogService struct {
	listQuery userloginlog.ListQuery
}

func (fakeRouterUserLoginLogService) PageInit(ctx context.Context) (*userloginlog.PageInitResponse, *apperror.Error) {
	return &userloginlog.PageInitResponse{}, nil
}

func (f *fakeRouterUserLoginLogService) List(ctx context.Context, query userloginlog.ListQuery) (*userloginlog.ListResponse, *apperror.Error) {
	f.listQuery = query
	return &userloginlog.ListResponse{
		List: []userloginlog.ListItem{{ID: 1, UserName: "admin", LoginType: "password"}},
		Page: userloginlog.Page{CurrentPage: query.CurrentPage, PageSize: query.PageSize, Total: 1, TotalPage: 1},
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
	return &user.ProfileResponse{Profile: user.ProfileDetail{UserID: userID, Username: "admin"}}, f.err
}

func (f *fakeRouterUserService) UpdateProfile(ctx context.Context, input user.UpdateProfileInput) *apperror.Error {
	f.profileUserID = input.UserID
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

func (f *fakeRouterPermissionService) Init(ctx context.Context) (*permission.InitResponse, *apperror.Error) {
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

func (f *fakeRouterRoleService) Init(ctx context.Context) (*role.InitResponse, *apperror.Error) {
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

func (f *fakeRouterAuthPlatformService) Init(ctx context.Context) (*authplatform.InitResponse, *apperror.Error) {
	return (&authplatform.Service{}).Init(ctx)
}

func (f *fakeRouterAuthPlatformService) List(ctx context.Context, query authplatform.ListQuery) (*authplatform.ListResponse, *apperror.Error) {
	f.listQuery = query
	return &authplatform.ListResponse{
		List: []authplatform.ListItem{{ID: 1, Code: "admin", Name: "PC后台", CaptchaType: captcha.TypeSlide}},
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

func (f *fakeRouterClientVersionService) Init(ctx context.Context) (*clientversion.InitResponse, *apperror.Error) {
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

func (f *fakeRouterAIProviderService) Init(ctx context.Context) (*aiprovider.InitResponse, *apperror.Error) {
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

func (f *fakeRouterAIProviderService) TestConnection(ctx context.Context, id uint64) (*platformai.TestConnectionResult, *apperror.Error) {
	f.testID = id
	return &platformai.TestConnectionResult{OK: true, Status: "200 OK", Message: "ok"}, nil
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

func (f *fakeRouterAIAgentService) Init(ctx context.Context) (*aiagent.InitResponse, *apperror.Error) {
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

func (f *fakeRouterAIAgentService) Test(ctx context.Context, id uint64) (*platformai.TestConnectionResult, *apperror.Error) {
	f.testID = id
	return &platformai.TestConnectionResult{OK: true, Status: "200 OK", Message: "ok"}, nil
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

func (f *fakeRouterAIToolService) Init(ctx context.Context) (*aitool.InitResponse, *apperror.Error) {
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

func (f *fakeRouterOperationLogService) Init(ctx context.Context) (*operationlog.InitResponse, *apperror.Error) {
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

func (f *fakeRouterNotificationService) Init(ctx context.Context) (*notification.InitResponse, *apperror.Error) {
	return notification.NewService(&fakeRepositoryForNotificationRouter{}).Init(ctx)
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

func (f *fakeRouterNotificationTaskService) Init(ctx context.Context) (*notificationtask.InitResponse, *apperror.Error) {
	return notificationtask.NewService(&fakeRepositoryForNotificationTaskRouter{}).Init(ctx)
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

func (f *fakeRouterCronTaskService) Init(ctx context.Context) (*crontask.InitResponse, *apperror.Error) {
	return crontask.NewService(&fakeCronTaskRepositoryForRouter{}, crontask.NewDefaultRegistry()).Init(ctx)
}

func (f *fakeRouterCronTaskService) List(ctx context.Context, query crontask.ListQuery) (*crontask.ListResponse, *apperror.Error) {
	f.listQuery = query
	return &crontask.ListResponse{
		List: []crontask.ListItem{{ID: 1, Name: "notification_task_scheduler", RegistryStatus: crontask.RegistryStatusRegistered}},
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
func (fakeCronTaskRepositoryForRouter) ListAll(ctx context.Context, query crontask.ListQuery) ([]crontask.Task, error) {
	return nil, nil
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

func (f *fakeRouterSystemLogService) Init(ctx context.Context) (*systemlog.InitResponse, *apperror.Error) {
	return systemlog.NewService(nil).Init(ctx)
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

func (f *fakeRouterSystemSettingService) Init(ctx context.Context) (*systemsetting.InitResponse, *apperror.Error) {
	return systemsetting.NewService(nil).Init(ctx)
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

type fakeRouterPaymentService struct {
	channelListQuery        payment.ChannelListQuery
	createChannelInput      payment.ChannelMutationInput
	updateChannelID         int64
	statusChannelID         int64
	status                  int
	deleteChannelID         int64
	orderListQuery          payment.OrderListQuery
	createOrderInput        payment.CreateOrderInput
	createOrderCalledUserID int64
	resultUserID            int64
	resultOrderNo           string
	payUserID               int64
	payOrderNo              string
	payReturnURL            string
	cancelUserID            int64
	cancelOrderNo           string
	adminOrderNo            string
	closeOrderNo            string
	eventListQuery          payment.EventListQuery
	eventID                 int64
	notifyInput             payment.NotifyInput
	notifyBody              string
}

func (f *fakeRouterPaymentService) ChannelInit(ctx context.Context) (*payment.ChannelInitResponse, *apperror.Error) {
	return payment.NewService(payment.Dependencies{}).ChannelInit(ctx)
}

func (f *fakeRouterPaymentService) ListChannels(ctx context.Context, query payment.ChannelListQuery) (*payment.ChannelListResponse, *apperror.Error) {
	f.channelListQuery = query
	return &payment.ChannelListResponse{List: []payment.ChannelListItem{{ID: 1, Name: "支付宝"}}, Page: payment.Page{CurrentPage: query.CurrentPage, PageSize: query.PageSize, Total: 1, TotalPage: 1}}, nil
}

func (f *fakeRouterPaymentService) CreateChannel(ctx context.Context, input payment.ChannelMutationInput) (int64, *apperror.Error) {
	f.createChannelInput = input
	return 11, nil
}

func (f *fakeRouterPaymentService) UpdateChannel(ctx context.Context, id int64, input payment.ChannelMutationInput) *apperror.Error {
	f.updateChannelID = id
	return nil
}

func (f *fakeRouterPaymentService) ChangeChannelStatus(ctx context.Context, id int64, status int) *apperror.Error {
	f.statusChannelID = id
	f.status = status
	return nil
}

func (f *fakeRouterPaymentService) DeleteChannel(ctx context.Context, id int64) *apperror.Error {
	f.deleteChannelID = id
	return nil
}

func (f *fakeRouterPaymentService) OrderInit(ctx context.Context) (*payment.ChannelInitResponse, *apperror.Error) {
	return payment.NewService(payment.Dependencies{}).OrderInit(ctx)
}

func (f *fakeRouterPaymentService) ListOrders(ctx context.Context, query payment.OrderListQuery) (*payment.OrderListResponse, *apperror.Error) {
	f.orderListQuery = query
	return &payment.OrderListResponse{List: []payment.OrderListItem{{ID: 1, OrderNo: "P1"}}, Page: payment.Page{CurrentPage: query.CurrentPage, PageSize: query.PageSize, Total: 1, TotalPage: 1}}, nil
}

func (f *fakeRouterPaymentService) GetAdminOrder(ctx context.Context, orderNo string) (*payment.OrderDetailResponse, *apperror.Error) {
	f.adminOrderNo = orderNo
	return &payment.OrderDetailResponse{Order: payment.OrderListItem{OrderNo: orderNo}}, nil
}

func (f *fakeRouterPaymentService) GetOrderResult(ctx context.Context, userID int64, orderNo string) (*payment.ResultResponse, *apperror.Error) {
	f.resultUserID = userID
	f.resultOrderNo = orderNo
	return &payment.ResultResponse{OrderNo: orderNo}, nil
}

func (f *fakeRouterPaymentService) CreateOrder(ctx context.Context, input payment.CreateOrderInput) (*payment.CreateOrderResponse, *apperror.Error) {
	f.createOrderInput = input
	f.createOrderCalledUserID = input.UserID
	return &payment.CreateOrderResponse{OrderNo: "P1", AmountCents: input.AmountCents}, nil
}

func (f *fakeRouterPaymentService) PayOrder(ctx context.Context, userID int64, orderNo string, returnURL string) (*payment.PayOrderResponse, *apperror.Error) {
	f.payUserID = userID
	f.payOrderNo = orderNo
	f.payReturnURL = returnURL
	return &payment.PayOrderResponse{OrderNo: orderNo, PayURL: "https://pay.example.test"}, nil
}

func (f *fakeRouterPaymentService) CancelOrder(ctx context.Context, userID int64, orderNo string) *apperror.Error {
	f.cancelUserID = userID
	f.cancelOrderNo = orderNo
	return nil
}

func (f *fakeRouterPaymentService) CloseAdminOrder(ctx context.Context, orderNo string) *apperror.Error {
	f.closeOrderNo = orderNo
	return nil
}

func (f *fakeRouterPaymentService) ListEvents(ctx context.Context, query payment.EventListQuery) (*payment.EventListResponse, *apperror.Error) {
	f.eventListQuery = query
	return &payment.EventListResponse{List: []payment.EventListItem{{ID: 1, OrderNo: query.OrderNo}}, Page: payment.Page{CurrentPage: query.CurrentPage, PageSize: query.PageSize, Total: 1, TotalPage: 1}}, nil
}

func (f *fakeRouterPaymentService) GetEvent(ctx context.Context, id int64) (*payment.EventDetailResponse, *apperror.Error) {
	f.eventID = id
	return &payment.EventDetailResponse{Event: payment.EventListItem{ID: id}}, nil
}

func (f *fakeRouterPaymentService) HandleAlipayNotify(ctx context.Context, input payment.NotifyInput) (string, *apperror.Error) {
	f.notifyInput = input
	if f.notifyBody != "" {
		return f.notifyBody, nil
	}
	return "success", nil
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
	router := newTestRouter(t, Dependencies{})

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
	router := newTestRouter(t, Dependencies{})

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
	router := newTestRouter(t, Dependencies{})

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
	router := newTestRouter(t, Dependencies{Readiness: fakeReadinessChecker{report: readiness.NewReport(map[string]readiness.Check{
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
	if body["msg"] != "service not ready" {
		t.Fatalf("expected service not ready message, got %#v", body["msg"])
	}

	data := mustRouterData(t, body)
	if data["status"] != readiness.StatusNotReady {
		t.Fatalf("expected not_ready status, got %#v", data["status"])
	}
}

func TestRouterInstallsAccessLogAfterRequestID(t *testing.T) {
	var buffer bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buffer, nil))
	router := NewRouter(Dependencies{Logger: logger})

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
	router := NewRouter(Dependencies{
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
	router := newTestRouter(t, Dependencies{})

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
	router := newTestRouter(t, Dependencies{AuthService: fakeAuthService{}})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/v1/auth/refresh", strings.NewReader(`{"refresh_token":"refresh-token"}`))
	request.Header.Set("Content-Type", "application/json")
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
	router := newTestRouter(t, Dependencies{
		CORS:        config.DefaultCORSConfig(),
		AuthService: fakeAuthService{},
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/v1/auth/refresh", strings.NewReader(`{"refresh_token":"refresh-token"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "http://127.0.0.1:5173")
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
	router := newTestRouter(t, Dependencies{AuthService: fakeAuthService{}})

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
	router.ServeHTTP(loginRecorder, loginRequest)
	if loginRecorder.Code != http.StatusOK {
		t.Fatalf("expected login status %d, got %d body=%s", http.StatusOK, loginRecorder.Code, loginRecorder.Body.String())
	}
}

func TestRouterInstallsCaptchaEndpointAsPublicPath(t *testing.T) {
	router := newTestRouter(t, Dependencies{CaptchaService: fakeCaptchaService{}})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/auth/captcha", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected captcha status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	body := decodeRouterBody(t, recorder)
	data := mustRouterData(t, body)
	if data["captcha_id"] != "captcha-id" || data["captcha_type"] != captcha.TypeSlide {
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
		QuickEntry:  []user.QuickEntry{{ID: 3, PermissionID: 2, Sort: 1}},
	}}
	router := newTestRouter(t, Dependencies{
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

func TestRouterInstallsUsersInitAsProtectedRESTPath(t *testing.T) {
	var authInput middleware.TokenInput
	userService := &fakeRouterUserService{result: &user.InitResponse{
		UserID:      1,
		Username:    "admin",
		Avatar:      "avatar.png",
		RoleName:    "管理员",
		Permissions: []permission.MenuItem{{Index: "1", Label: "系统", Children: []permission.MenuItem{}}},
		Router:      []permission.RouteItem{{Name: "menu_2", Path: "/system/user", ViewKey: "system/user/index"}},
		ButtonCodes: []string{"user_add"},
		QuickEntry:  []user.QuickEntry{{ID: 3, PermissionID: 2, Sort: 1}},
	}}
	router := newTestRouter(t, Dependencies{
		Authenticator: func(ctx context.Context, input middleware.TokenInput) (*middleware.AuthIdentity, *apperror.Error) {
			authInput = input
			return &middleware.AuthIdentity{UserID: 1, SessionID: 10, Platform: input.Platform}, nil
		},
		UserService: userService,
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/users/init", nil)
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
		t.Fatalf("unexpected users/init payload: %#v", data)
	}
	if _, ok := data["buttonCodes"]; !ok {
		t.Fatalf("missing buttonCodes in users/init payload: %#v", data)
	}
}

func TestRouterInstallsUserManagementRESTRoutes(t *testing.T) {
	userService := &fakeRouterUserService{}
	router := newTestRouter(t, Dependencies{
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
	userSessionService := &fakeRouterUserSessionService{}
	router := newTestRouter(t, Dependencies{
		Authenticator: func(ctx context.Context, input middleware.TokenInput) (*middleware.AuthIdentity, *apperror.Error) {
			return &middleware.AuthIdentity{UserID: 1, SessionID: 10, Platform: "admin"}, nil
		},
		UserSessionService: userSessionService,
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
	query := userSessionService.listQuery
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
	quickEntryService := &fakeRouterUserQuickEntryService{}
	loginLogService := &fakeRouterUserLoginLogService{}
	userSessionService := &fakeRouterUserSessionService{}
	router := newTestRouter(t, Dependencies{
		Authenticator: func(ctx context.Context, input middleware.TokenInput) (*middleware.AuthIdentity, *apperror.Error) {
			return &middleware.AuthIdentity{UserID: 44, SessionID: 55, Platform: "admin"}, nil
		},
		UserQuickEntryService: quickEntryService,
		UserLoginLogService:   loginLogService,
		UserSessionService:    userSessionService,
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/api/admin/v1/users/me/quick-entries", strings.NewReader(`{"permission_ids":[3,1,3]}`))
	request.Header.Set("Authorization", "Bearer access-token")
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || quickEntryService.userID != 44 || !reflect.DeepEqual(quickEntryService.input.PermissionIDs, []int64{3, 1, 3}) {
		t.Fatalf("expected quick-entry route, code=%d body=%s service=%#v", recorder.Code, recorder.Body.String(), quickEntryService)
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
	if recorder.Code != http.StatusOK || userSessionService.revokeID != 77 || userSessionService.currentSession != 55 {
		t.Fatalf("expected session revoke route, code=%d body=%s service=%#v", recorder.Code, recorder.Body.String(), userSessionService)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPatch, "/api/admin/v1/user-sessions/revoke", strings.NewReader(`{"ids":[77,78]}`))
	request.Header.Set("Authorization", "Bearer access-token")
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || userSessionService.currentSession != 55 || !reflect.DeepEqual(userSessionService.batchInput.IDs, []int64{77, 78}) {
		t.Fatalf("expected session batch revoke route, code=%d body=%s service=%#v", recorder.Code, recorder.Body.String(), userSessionService)
	}
}

func TestRouterInstallsExportTaskRESTRoutes(t *testing.T) {
	exportTaskService := &fakeRouterExportTaskService{}
	router := newTestRouter(t, Dependencies{
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
	router := newTestRouter(t, Dependencies{
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
	router := newTestRouter(t, Dependencies{
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
	router := newTestRouter(t, Dependencies{
		Authenticator: func(ctx context.Context, input middleware.TokenInput) (*middleware.AuthIdentity, *apperror.Error) {
			return &middleware.AuthIdentity{UserID: 12, SessionID: 10, Platform: "admin"}, nil
		},
		NotificationTaskService: notificationTaskService,
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/notification-tasks/init", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected notification task init status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
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
	router := newTestRouter(t, Dependencies{
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
	router := newTestRouter(t, Dependencies{
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
	router := newTestRouter(t, Dependencies{
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
	router := newTestRouter(t, Dependencies{
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
	router := newTestRouter(t, Dependencies{
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
	request = httptest.NewRequest(http.MethodPost, "/api/admin/v1/ai-providers/model-options", strings.NewReader(`{"driver":"openai","api_key":"sk-test"}`))
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
	router := newTestRouter(t, Dependencies{
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
	router := newTestRouter(t, Dependencies{
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
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/cron-tasks?current_page=1&page_size=20&status=1&registry_status=registered&title=通知", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected cron task list status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if cronTaskService.listQuery.CurrentPage != 1 || cronTaskService.listQuery.PageSize != 20 || cronTaskService.listQuery.RegistryStatus != crontask.RegistryStatusRegistered || cronTaskService.listQuery.Title != "通知" {
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
	router := newTestRouter(t, Dependencies{
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
	router := newTestRouter(t, Dependencies{
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
func TestRouterInstallsPaymentRoutesAndRawNotify(t *testing.T) {
	paymentService := &fakeRouterPaymentService{notifyBody: "success"}
	router := newTestRouter(t, Dependencies{
		Authenticator: func(ctx context.Context, input middleware.TokenInput) (*middleware.AuthIdentity, *apperror.Error) {
			return &middleware.AuthIdentity{UserID: 7, SessionID: 10, Platform: "admin"}, nil
		},
		PermissionRules: map[middleware.RouteKey]string{
			middleware.NewRouteKey(http.MethodGet, "/api/admin/v1/payment/channels"):  "payment_channel_list",
			middleware.NewRouteKey(http.MethodPost, "/api/admin/v1/payment/channels"): "payment_channel_add",
			middleware.NewRouteKey(http.MethodGet, "/api/admin/v1/payment/orders"):    "payment_order_list",
			middleware.NewRouteKey(http.MethodGet, "/api/admin/v1/payment/events"):    "payment_event_list",
		},
		PermissionChecker: func(ctx context.Context, input middleware.PermissionInput) *apperror.Error { return nil },
		PaymentService:    paymentService,
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/payment/channels?current_page=2&page_size=30&name=ali&provider=alipay&status=1", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected payment channel list status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if paymentService.channelListQuery.CurrentPage != 2 || paymentService.channelListQuery.Provider != "alipay" || paymentService.channelListQuery.Status != 1 {
		t.Fatalf("payment channel list query mismatch: %#v", paymentService.channelListQuery)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/admin/v1/payment/orders", strings.NewReader(`{"channel_id":1,"pay_method":"web","subject":"test","amount_cents":100}`))
	request.Header.Set("Authorization", "Bearer access-token")
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || paymentService.createOrderCalledUserID != 7 || paymentService.createOrderInput.AmountCents != 100 {
		t.Fatalf("expected payment create order route, code=%d body=%s service=%#v", recorder.Code, recorder.Body.String(), paymentService)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/admin/v1/payment/orders/P1/pay", strings.NewReader(`{"return_url":"https://example.test/return"}`))
	request.Header.Set("Authorization", "Bearer access-token")
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || paymentService.payUserID != 7 || paymentService.payOrderNo != "P1" || paymentService.payReturnURL != "https://example.test/return" {
		t.Fatalf("expected payment pay order route, code=%d body=%s service=%#v", recorder.Code, recorder.Body.String(), paymentService)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/admin/v1/payment/events?current_page=1&page_size=20&order_no=P1&event_type=notify&process_status=1", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || paymentService.eventListQuery.OrderNo != "P1" || paymentService.eventListQuery.EventType != "notify" {
		t.Fatalf("expected payment event list route, code=%d body=%s query=%#v", recorder.Code, recorder.Body.String(), paymentService.eventListQuery)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/payment/notify/alipay", strings.NewReader(`out_trade_no=P1&trade_no=A1`))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected notify status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if strings.TrimSpace(recorder.Body.String()) != "success" || strings.Contains(recorder.Body.String(), `"code"`) {
		t.Fatalf("notify must return raw text/plain service body, got content-type=%q body=%q", recorder.Header().Get("Content-Type"), recorder.Body.String())
	}
	if got := recorder.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/plain") {
		t.Fatalf("notify must return text/plain, got %q", got)
	}
	if paymentService.notifyInput.Form["out_trade_no"] != "P1" {
		t.Fatalf("notify form mismatch: %#v", paymentService.notifyInput.Form)
	}
}
func TestRouterInstallsSystemLogReadOnlyRESTRoutes(t *testing.T) {
	systemLogService := &fakeRouterSystemLogService{}
	router := newTestRouter(t, Dependencies{
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
	router := newTestRouter(t, Dependencies{
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
	var uiAuthToken string
	var uiAuthPlatform string
	router := newTestRouter(t, Dependencies{
		Authenticator: func(ctx context.Context, input middleware.TokenInput) (*middleware.AuthIdentity, *apperror.Error) {
			if strings.HasPrefix(input.AccessToken, "cookie-") {
				uiAuthToken = input.AccessToken
				uiAuthPlatform = input.Platform
			}
			return &middleware.AuthIdentity{UserID: 1, SessionID: 10, Platform: "admin"}, nil
		},
		QueueMonitorService: queueMonitorService,
		QueueMonitorUI:      queueMonitorUI,
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
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected queue monitor UI status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if !queueMonitorUI.called || queueMonitorUI.path != queuemonitor.UIPath+"/api/queues" || queueMonitorUI.method != http.MethodGet {
		t.Fatalf("queue monitor UI handler not called as expected: %#v", queueMonitorUI)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, queuemonitor.UIPath, nil)
	request.AddCookie(&http.Cookie{Name: middleware.DefaultAccessTokenCookie, Value: "cookie-access-token"})
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected queue monitor UI cookie status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if uiAuthToken != "cookie-access-token" {
		t.Fatalf("expected queue monitor UI to authenticate with cookie token, got %q", uiAuthToken)
	}
	if uiAuthPlatform != "admin" {
		t.Fatalf("expected queue monitor UI cookie auth to use admin platform, got %q", uiAuthPlatform)
	}
}

func TestRouterInstallsPermissionCheckAfterAuthToken(t *testing.T) {
	router := newTestRouter(t, Dependencies{
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
	router := newTestRouter(t, Dependencies{
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
	router := newTestRouter(t, Dependencies{
		Authenticator: func(ctx context.Context, input middleware.TokenInput) (*middleware.AuthIdentity, *apperror.Error) {
			return &middleware.AuthIdentity{UserID: 7, SessionID: 9, Platform: "admin"}, nil
		},
		RealtimeHandler: realtimemodule.NewHandler(
			realtimemodule.NewService(25*time.Second),
			platformrealtime.NewUpgrader(func(*http.Request) bool { return true }),
			platformrealtime.NewManager(),
			slog.New(slog.NewTextHandler(io.Discard, nil)),
		),
	})
	server := httptest.NewServer(router)
	defer server.Close()

	client, _, err := websocket.DefaultDialer.Dial("ws"+server.URL[len("http"):]+"/api/admin/v1/realtime/ws", http.Header{
		"Authorization": []string{"Bearer access-token"},
		"platform":      []string{"admin"},
		"device-id":     []string{"codex-test"},
	})
	if err != nil {
		t.Fatalf("dial realtime: %v", err)
	}
	defer client.Close()

	var connected map[string]any
	if err := client.ReadJSON(&connected); err != nil {
		t.Fatalf("read connected: %v", err)
	}
	if connected["type"] != realtimemodule.TypeConnectedV1 {
		t.Fatalf("expected connected event, got %#v", connected)
	}
}

func TestRealtimeRouteAcceptsPathScopedCookieTokenForBrowserWebSocket(t *testing.T) {
	var gotInput middleware.TokenInput
	router := newTestRouter(t, Dependencies{
		Authenticator: func(ctx context.Context, input middleware.TokenInput) (*middleware.AuthIdentity, *apperror.Error) {
			gotInput = input
			return &middleware.AuthIdentity{UserID: 7, SessionID: 9, Platform: input.Platform}, nil
		},
		RealtimeHandler: realtimemodule.NewHandler(
			realtimemodule.NewService(25*time.Second),
			platformrealtime.NewUpgrader(func(*http.Request) bool { return true }),
			platformrealtime.NewManager(),
			slog.New(slog.NewTextHandler(io.Discard, nil)),
		),
	})
	server := httptest.NewServer(router)
	defer server.Close()

	client, _, err := websocket.DefaultDialer.Dial("ws"+server.URL[len("http"):]+realtimemodule.WSPath, http.Header{
		"Cookie": []string{middleware.DefaultAccessTokenCookie + "=cookie-access-token"},
	})
	if err != nil {
		t.Fatalf("dial realtime with cookie token: %v", err)
	}
	defer client.Close()

	var connected map[string]any
	if err := client.ReadJSON(&connected); err != nil {
		t.Fatalf("read connected: %v", err)
	}
	if connected["type"] != realtimemodule.TypeConnectedV1 {
		t.Fatalf("expected connected event, got %#v", connected)
	}
	if gotInput.AccessToken != "cookie-access-token" {
		t.Fatalf("expected cookie access token, got %q", gotInput.AccessToken)
	}
	if gotInput.Platform != "admin" {
		t.Fatalf("expected cookie websocket auth to default platform admin, got %q", gotInput.Platform)
	}
}

func TestRealtimeRouteAllowsConfiguredBrowserOrigin(t *testing.T) {
	router := newTestRouter(t, Dependencies{
		CORS: config.CORSConfig{
			AllowOrigins:     []string{"http://127.0.0.1:5173"},
			AllowMethods:     []string{"GET", "OPTIONS"},
			AllowHeaders:     []string{"Authorization", "platform", "device-id"},
			AllowCredentials: true,
		},
		Authenticator: func(ctx context.Context, input middleware.TokenInput) (*middleware.AuthIdentity, *apperror.Error) {
			return &middleware.AuthIdentity{UserID: 7, SessionID: 9, Platform: input.Platform}, nil
		},
		RealtimeHandler: realtimemodule.NewHandler(
			realtimemodule.NewService(25*time.Second),
			platformrealtime.NewUpgrader(platformrealtime.NewAllowedOriginChecker([]string{"http://127.0.0.1:5173"})),
			platformrealtime.NewManager(),
			slog.New(slog.NewTextHandler(io.Discard, nil)),
		),
	})
	server := httptest.NewServer(router)
	defer server.Close()

	client, _, err := websocket.DefaultDialer.Dial("ws"+server.URL[len("http"):]+realtimemodule.WSPath, http.Header{
		"Cookie": []string{middleware.DefaultAccessTokenCookie + "=cookie-access-token"},
		"Origin": []string{"http://127.0.0.1:5173"},
	})
	if err != nil {
		t.Fatalf("dial realtime from configured origin: %v", err)
	}
	defer client.Close()

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
	router := newTestRouter(t, Dependencies{
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
	router := newTestRouter(t, Dependencies{
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
		{http.MethodGet, "/api/admin/v1/ai-prompts", ""},
		{http.MethodPost, "/api/admin/v1/ai-prompts", `{"title":"prompt","content":"text"}`},
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

func TestRouterInstallsAIRuntimeRESTRoutes(t *testing.T) {
	router := newTestRouter(t, Dependencies{
		Authenticator: func(ctx context.Context, input middleware.TokenInput) (*middleware.AuthIdentity, *apperror.Error) {
			return &middleware.AuthIdentity{UserID: 7, SessionID: 9, Platform: "admin"}, nil
		},
		AiConversationService: fakeRouterAIConversationService{},
		AiMessageService:      fakeRouterAIMessageService{},
		AiRunService:          fakeRouterAIRunService{},
		AiChatService:         fakeRouterAIChatService{},
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

func newTestRouter(t *testing.T, deps Dependencies) http.Handler {
	t.Helper()
	if deps.Logger == nil {
		deps.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return NewRouter(deps)
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

func assertRequestID(t *testing.T, recorder *httptest.ResponseRecorder) {
	t.Helper()
	if recorder.Header().Get("X-Request-Id") == "" {
		t.Fatalf("expected X-Request-Id header")
	}
}
