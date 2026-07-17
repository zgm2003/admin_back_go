package asset

import (
	"context"
	"errors"
	"testing"

	"admin_back_go/internal/shared/apperror"
)

type fakeAssetRepository struct {
	rows         []Asset
	err          error
	listQuery    ListQuery
	created      Asset
	updatedID    int64
	updated      Asset
	deletedID    int64
	deletedUser  uint64
	createCalled bool
	updateCalled bool
	deleteCalled bool
}

func (f *fakeAssetRepository) List(ctx context.Context, query ListQuery) ([]Asset, int64, error) {
	f.listQuery = query
	return f.rows, int64(len(f.rows)), f.err
}

func (f *fakeAssetRepository) Create(ctx context.Context, row Asset) (int64, error) {
	f.created = row
	f.createCalled = true
	return 11, f.err
}

func (f *fakeAssetRepository) Update(ctx context.Context, id int64, row Asset) error {
	f.updatedID = id
	f.updated = row
	f.updateCalled = true
	return f.err
}

func (f *fakeAssetRepository) SoftDelete(ctx context.Context, id int64, userID uint64) error {
	f.deletedID = id
	f.deletedUser = userID
	f.deleteCalled = true
	return f.err
}

func TestServiceUserListForcesEnabledActiveRowsAndOwner(t *testing.T) {
	repo := &fakeAssetRepository{rows: []Asset{{ID: 1, UserID: 9, Slug: "hero", Type: AssetTypeImage, Title: "Hero", Status: StatusEnabled, IsDel: IsDelActive}}}
	svc := NewService(repo)

	result, appErr := svc.UserList(context.Background(), 9, ListQuery{Status: StatusDisabled, IsDel: IsDelDeleted})

	if appErr != nil {
		t.Fatalf("UserList returned error: %#v", appErr)
	}
	if len(result.List) != 1 || result.List[0].UserID != 9 {
		t.Fatalf("expected one user-owned asset, got %#v", result)
	}
	if repo.listQuery.UserID != 9 || repo.listQuery.Status != StatusEnabled || repo.listQuery.IsDel != IsDelActive {
		t.Fatalf("UserList must force owner/enabled/active rows, query=%#v", repo.listQuery)
	}
}

func TestServiceUserCreateRejectsMissingOwnerAndInvalidFields(t *testing.T) {
	svc := NewService(&fakeAssetRepository{})
	for _, input := range []Input{
		{UserID: 0, Slug: "slug", Type: AssetTypeImage, Title: "Title"},
		{UserID: 9, Slug: "", Type: AssetTypeText, Title: "Title"},
		{UserID: 9, Slug: "slug", Type: "", Title: "Title"},
		{UserID: 9, Slug: "slug", Type: AssetTypeImage, Title: ""},
		{UserID: 9, Slug: "slug", Type: "audio", Title: "Title"},
		{UserID: 9, Slug: "slug", Type: AssetTypeImage, Title: "Title", Status: 999},
	} {
		_, appErr := svc.Create(context.Background(), input)
		if appErr == nil || appErr.LegacyCode != apperror.CodeBadRequest {
			t.Fatalf("expected CodeBadRequest for %#v, got %#v", input, appErr)
		}
	}
}

func TestServiceUserCreateRejectsMediaAssetsWithoutStrictMetadata(t *testing.T) {
	for name, input := range map[string]Input{
		"image missing content":      {UserID: 9, Slug: "hero", Type: AssetTypeImage, Title: "Hero", URL: "https://storage.example.test/hero.png"},
		"image invalid json":         {UserID: 9, Slug: "hero", Type: AssetTypeImage, Title: "Hero", URL: "https://storage.example.test/hero.png", Content: `{`},
		"image missing storage key":  {UserID: 9, Slug: "hero", Type: AssetTypeImage, Title: "Hero", URL: "https://storage.example.test/hero.png", Content: `{"width":1024,"height":768,"bytes":123,"mimeType":"image/png"}`},
		"image bare storage key":     {UserID: 9, Slug: "hero", Type: AssetTypeImage, Title: "Hero", URL: "https://storage.example.test/hero.png", Content: `{"storageKey":"localBrowserOnly","width":1024,"height":768,"bytes":123,"mimeType":"image/png"}`},
		"image wrong storage prefix": {UserID: 9, Slug: "hero", Type: AssetTypeImage, Title: "Hero", URL: "https://storage.example.test/hero.png", Content: `{"storageKey":"video:task/hero.png","width":1024,"height":768,"bytes":123,"mimeType":"image/png"}`},
		"image local browser key":    {UserID: 9, Slug: "hero", Type: AssetTypeImage, Title: "Hero", URL: "https://storage.example.test/hero.png", Content: `{"storageKey":"image:localBrowserOnly","width":1024,"height":768,"bytes":123,"mimeType":"image/png"}`},
		"image wrong mime":           {UserID: 9, Slug: "hero", Type: AssetTypeImage, Title: "Hero", URL: "https://storage.example.test/hero.png", Content: `{"storageKey":"image:task/hero.png","width":1024,"height":768,"bytes":123,"mimeType":"video/mp4"}`},
		"image zero bytes":           {UserID: 9, Slug: "hero", Type: AssetTypeImage, Title: "Hero", URL: "https://storage.example.test/hero.png", Content: `{"storageKey":"image:task/hero.png","width":1024,"height":768,"bytes":0,"mimeType":"image/png"}`},
		"video missing url":          {UserID: 9, Slug: "clip", Type: AssetTypeVideo, Title: "Clip", Content: `{"storageKey":"video:task/clip.mp4","width":1280,"height":720,"bytes":456,"mimeType":"video/mp4"}`},
		"video wrong storage prefix": {UserID: 9, Slug: "clip", Type: AssetTypeVideo, Title: "Clip", URL: "https://storage.example.test/clip.mp4", Content: `{"storageKey":"image:task/clip.mp4","width":1280,"height":720,"bytes":456,"mimeType":"video/mp4"}`},
		"video wrong mime":           {UserID: 9, Slug: "clip", Type: AssetTypeVideo, Title: "Clip", URL: "https://storage.example.test/clip.mp4", Content: `{"storageKey":"video:task/clip.mp4","width":1280,"height":720,"bytes":456,"mimeType":"image/png"}`},
		"video zero height":          {UserID: 9, Slug: "clip", Type: AssetTypeVideo, Title: "Clip", URL: "https://storage.example.test/clip.mp4", Content: `{"storageKey":"video:task/clip.mp4","width":1280,"height":0,"bytes":456,"mimeType":"video/mp4"}`},
		"video unknown metadata key": {UserID: 9, Slug: "clip", Type: AssetTypeVideo, Title: "Clip", URL: "https://storage.example.test/clip.mp4", Content: `{"storageKey":"video:task/clip.mp4","width":1280,"height":720,"bytes":456,"mimeType":"video/mp4","provider":"browser"}`},
	} {
		repo := &fakeAssetRepository{}
		svc := NewService(repo)
		if _, appErr := svc.Create(context.Background(), input); appErr == nil || appErr.LegacyCode != apperror.CodeBadRequest {
			t.Fatalf("%s: expected CodeBadRequest, got %#v", name, appErr)
		}
		if repo.createCalled {
			t.Fatalf("%s: invalid media metadata must not reach repository, row=%#v", name, repo.created)
		}
	}
}

func TestServiceUserCreateAcceptsMediaAssetsWithStrictMetadata(t *testing.T) {
	repo := &fakeAssetRepository{}
	svc := NewService(repo)

	id, appErr := svc.UserCreate(context.Background(), 9, validVideoInput())

	if appErr != nil || id != 11 {
		t.Fatalf("UserCreate returned id=%d err=%#v", id, appErr)
	}
	if !repo.createCalled || repo.created.UserID != 9 || repo.created.Type != AssetTypeVideo || repo.created.Status != StatusDisabled || repo.created.IsDel != IsDelActive || repo.created.Content == "" || repo.created.URL == "" {
		t.Fatalf("unexpected created asset: called=%v row=%#v", repo.createCalled, repo.created)
	}
}

func TestServiceUserUpdateAndDeleteSurfaceRepositoryErrorsAsInternal(t *testing.T) {
	repoErr := errors.New("db down")
	svc := NewService(&fakeAssetRepository{err: repoErr})

	_, appErr := svc.UserList(context.Background(), 9, ListQuery{})
	if appErr == nil || appErr.LegacyCode != apperror.CodeInternal || !errors.Is(appErr, repoErr) {
		t.Fatalf("expected wrapped CodeInternal list error, got %#v", appErr)
	}

	_, appErr = svc.UserCreate(context.Background(), 9, validImageInput())
	if appErr == nil || appErr.LegacyCode != apperror.CodeInternal || !errors.Is(appErr, repoErr) {
		t.Fatalf("expected wrapped CodeInternal create error, got %#v", appErr)
	}

	appErr = svc.UserUpdate(context.Background(), 9, 5, validImageInput())
	if appErr == nil || appErr.LegacyCode != apperror.CodeInternal || !errors.Is(appErr, repoErr) {
		t.Fatalf("expected wrapped CodeInternal update error, got %#v", appErr)
	}

	appErr = svc.UserDelete(context.Background(), 9, 5)
	if appErr == nil || appErr.LegacyCode != apperror.CodeInternal || !errors.Is(appErr, repoErr) {
		t.Fatalf("expected wrapped CodeInternal delete error, got %#v", appErr)
	}
}

func TestServiceUserUpdateAndDeleteRejectInvalidIDOrOwner(t *testing.T) {
	repo := &fakeAssetRepository{}
	svc := NewService(repo)

	if _, appErr := svc.UserList(context.Background(), 0, ListQuery{}); appErr == nil || appErr.LegacyCode != apperror.CodeBadRequest {
		t.Fatalf("expected bad request list owner, got %#v", appErr)
	}
	if appErr := svc.UserUpdate(context.Background(), 9, 0, validImageInput()); appErr == nil || appErr.LegacyCode != apperror.CodeBadRequest {
		t.Fatalf("expected bad request update id, got %#v", appErr)
	}
	invalidStatusInput := validImageInput()
	invalidStatusInput.Status = -1
	if appErr := svc.UserUpdate(context.Background(), 9, 5, invalidStatusInput); appErr == nil || appErr.LegacyCode != apperror.CodeBadRequest {
		t.Fatalf("expected bad request update status, got %#v", appErr)
	}
	if appErr := svc.UserDelete(context.Background(), 0, 5); appErr == nil || appErr.LegacyCode != apperror.CodeBadRequest {
		t.Fatalf("expected bad request delete owner, got %#v", appErr)
	}
	if appErr := svc.UserDelete(context.Background(), 9, 0); appErr == nil || appErr.LegacyCode != apperror.CodeBadRequest {
		t.Fatalf("expected bad request delete id, got %#v", appErr)
	}
	if repo.updateCalled || repo.deleteCalled {
		t.Fatalf("invalid input must not reach repository: %#v", repo)
	}
}

func TestServicePageInitKeepsAssetTypeDictionary(t *testing.T) {
	svc := NewService(&fakeAssetRepository{})

	initResult, appErr := svc.PageInit(context.Background())
	if appErr != nil || len(initResult.CommonStatusArr) == 0 || len(initResult.AIAssetTypeArr) != 3 {
		t.Fatalf("expected page init options, result=%#v err=%#v", initResult, appErr)
	}
}

func TestServiceRejectsInvalidAssetListStatus(t *testing.T) {
	svc := NewService(&fakeAssetRepository{})

	if _, appErr := svc.List(context.Background(), ListQuery{UserID: 9, Status: 999}); appErr == nil || appErr.LegacyCode != apperror.CodeBadRequest {
		t.Fatalf("expected bad request list status, got %#v", appErr)
	}
}

func validImageInput() Input {
	return Input{
		Slug:     "hero",
		Type:     AssetTypeImage,
		Title:    "Hero",
		URL:      "https://storage.example.test/task/hero.png",
		Content:  `{"storageKey":"ai-images/2026/06/09/hero.png","width":1024,"height":768,"bytes":123456,"mimeType":"image/png"}`,
		TagsJSON: `[]`,
	}
}

func validVideoInput() Input {
	return Input{
		Slug:     "clip",
		Type:     AssetTypeVideo,
		Title:    "Clip",
		URL:      "https://storage.example.test/task/clip.mp4",
		Content:  `{"storageKey":"video:task/clip.mp4","width":1280,"height":720,"bytes":456789,"mimeType":"video/mp4","duration":12.5}`,
		TagsJSON: `[]`,
		Status:   StatusDisabled,
	}
}
