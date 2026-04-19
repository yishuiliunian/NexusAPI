package oauth

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	doauth "github.com/yishuiliunian/nexusapi/backend/internal/domain/oauth"
	"github.com/yishuiliunian/nexusapi/backend/internal/domain/user"
	derrors "github.com/yishuiliunian/nexusapi/backend/pkg/errors"
)

// ---------- fakes ----------

type fakeBindings struct {
	mu     sync.Mutex
	byKey  map[string]*doauth.Binding // provider:external
	nextID uint64
}

func newFakeBindings() *fakeBindings {
	return &fakeBindings{byKey: map[string]*doauth.Binding{}, nextID: 1}
}
func (f *fakeBindings) Create(_ context.Context, b *doauth.Binding) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	b.ID = f.nextID
	f.nextID++
	cp := *b
	f.byKey[b.Provider+":"+b.ExternalID] = &cp
	return nil
}
func (f *fakeBindings) GetByProviderExternal(_ context.Context, p, eid string) (*doauth.Binding, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if b, ok := f.byKey[p+":"+eid]; ok {
		cp := *b
		return &cp, nil
	}
	return nil, derrors.ErrNotFound
}
func (f *fakeBindings) ListByUser(_ context.Context, uid uint64) ([]*doauth.Binding, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []*doauth.Binding{}
	for _, b := range f.byKey {
		if b.UserID == uid {
			cp := *b
			out = append(out, &cp)
		}
	}
	return out, nil
}
func (f *fakeBindings) Delete(_ context.Context, id uint64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for k, b := range f.byKey {
		if b.ID == id {
			delete(f.byKey, k)
			return nil
		}
	}
	return derrors.ErrNotFound
}

type fakeUsers struct {
	mu      sync.Mutex
	byID    map[uint64]*user.User
	byEmail map[string]*user.User
	nextID  uint64
}

func newFakeUsers() *fakeUsers {
	return &fakeUsers{byID: map[uint64]*user.User{}, byEmail: map[string]*user.User{}, nextID: 1}
}
func (f *fakeUsers) Create(_ context.Context, u *user.User) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	u.ID = f.nextID
	f.nextID++
	cp := *u
	f.byID[u.ID] = &cp
	f.byEmail[u.Email] = &cp
	return nil
}
func (f *fakeUsers) GetByID(_ context.Context, id uint64) (*user.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if u, ok := f.byID[id]; ok {
		cp := *u
		return &cp, nil
	}
	return nil, derrors.ErrNotFound
}
func (f *fakeUsers) GetByEmail(_ context.Context, email string) (*user.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if u, ok := f.byEmail[email]; ok {
		cp := *u
		return &cp, nil
	}
	return nil, derrors.ErrNotFound
}
func (f *fakeUsers) Update(_ context.Context, u *user.User) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	stored, ok := f.byID[u.ID]
	if !ok {
		return derrors.ErrNotFound
	}
	*stored = *u
	return nil
}
func (f *fakeUsers) List(_ context.Context, _, _ int) ([]*user.User, int64, error) {
	return nil, 0, nil
}
func (f *fakeUsers) SetQuota(_ context.Context, _ uint64, _ int64) error { return nil }
func (f *fakeUsers) ListLowQuotaForAlert(_ context.Context, _ time.Time, _ int) ([]*user.User, error) {
	return nil, nil
}

type fakeProvider struct {
	name    string
	profile *Profile
	err     error
}

func (f *fakeProvider) Name() string { return f.name }
func (f *fakeProvider) AuthorizeURL(state, redirectURI string) string {
	return "https://fake/authorize?state=" + state
}
func (f *fakeProvider) Exchange(_ context.Context, _, _ string) (*Profile, error) {
	return f.profile, f.err
}

// ---------- Tests ----------

func TestProviders(t *testing.T) {
	svc := NewService(newFakeBindings(), newFakeUsers(),
		&fakeProvider{name: "github"}, &fakeProvider{name: "google"})
	names := svc.Providers()
	if len(names) != 2 {
		t.Errorf("providers: %+v", names)
	}
}

func TestStartAuthorize_UnknownProvider(t *testing.T) {
	svc := NewService(newFakeBindings(), newFakeUsers(), &fakeProvider{name: "github"})
	_, _, err := svc.StartAuthorize("unknown", "cb")
	if !derrors.Is(err, derrors.CodeNotFound) {
		t.Errorf("want NotFound, got %v", err)
	}
}

func TestStartAuthorize_ReturnsURLAndState(t *testing.T) {
	svc := NewService(newFakeBindings(), newFakeUsers(), &fakeProvider{name: "github"})
	url, state, err := svc.StartAuthorize("github", "cb")
	if err != nil {
		t.Fatal(err)
	}
	if url == "" || state == "" {
		t.Errorf("url=%q state=%q", url, state)
	}
}

func TestStartAuthorize_StateUnique(t *testing.T) {
	svc := NewService(newFakeBindings(), newFakeUsers(), &fakeProvider{name: "github"})
	_, s1, _ := svc.StartAuthorize("github", "cb")
	_, s2, _ := svc.StartAuthorize("github", "cb")
	if s1 == s2 {
		t.Error("state 不唯一")
	}
}

func TestHandleCallback_NewUser(t *testing.T) {
	bindings := newFakeBindings()
	users := newFakeUsers()
	prov := &fakeProvider{name: "github", profile: &Profile{ExternalID: "42", Email: "alice@x.com", Name: "Alice"}}
	svc := NewService(bindings, users, prov)

	u, err := svc.HandleCallback(context.Background(), "github", "code", "cb")
	if err != nil {
		t.Fatalf("%v", err)
	}
	if u.Email != "alice@x.com" || !u.EmailVerified {
		t.Errorf("%+v", u)
	}
	// binding 已登记
	b, err := bindings.GetByProviderExternal(context.Background(), "github", "42")
	if err != nil || b.UserID != u.ID {
		t.Errorf("binding: %+v %v", b, err)
	}
}

func TestHandleCallback_ExistingBinding(t *testing.T) {
	bindings := newFakeBindings()
	users := newFakeUsers()
	existing := &user.User{Email: "bob@x.com"}
	_ = users.Create(context.Background(), existing)
	_ = bindings.Create(context.Background(), &doauth.Binding{UserID: existing.ID, Provider: "github", ExternalID: "99"})

	prov := &fakeProvider{name: "github", profile: &Profile{ExternalID: "99", Email: "bob@x.com"}}
	svc := NewService(bindings, users, prov)

	u, err := svc.HandleCallback(context.Background(), "github", "code", "cb")
	if err != nil {
		t.Fatalf("%v", err)
	}
	if u.ID != existing.ID {
		t.Errorf("应返回已有 user: %d vs %d", u.ID, existing.ID)
	}
}

func TestHandleCallback_MergeByEmail(t *testing.T) {
	bindings := newFakeBindings()
	users := newFakeUsers()
	existing := &user.User{Email: "merge@x.com"}
	_ = users.Create(context.Background(), existing)

	prov := &fakeProvider{name: "github", profile: &Profile{ExternalID: "77", Email: "merge@x.com"}}
	svc := NewService(bindings, users, prov)

	u, err := svc.HandleCallback(context.Background(), "github", "code", "cb")
	if err != nil {
		t.Fatalf("%v", err)
	}
	if u.ID != existing.ID {
		t.Errorf("应复用 email 匹配的 user: %d vs %d", u.ID, existing.ID)
	}
	b, _ := bindings.GetByProviderExternal(context.Background(), "github", "77")
	if b == nil || b.UserID != existing.ID {
		t.Error("binding 应挂到 existing user")
	}
}

func TestHandleCallback_ExchangeFails(t *testing.T) {
	prov := &fakeProvider{name: "github", err: errors.New("invalid_grant")}
	svc := NewService(newFakeBindings(), newFakeUsers(), prov)
	_, err := svc.HandleCallback(context.Background(), "github", "code", "cb")
	if !derrors.Is(err, derrors.CodeUpstream) {
		t.Errorf("want Upstream, got %v", err)
	}
}

func TestHandleCallback_NoExternalID(t *testing.T) {
	prov := &fakeProvider{name: "github", profile: &Profile{ExternalID: "", Email: "x@x"}}
	svc := NewService(newFakeBindings(), newFakeUsers(), prov)
	_, err := svc.HandleCallback(context.Background(), "github", "code", "cb")
	if !derrors.Is(err, derrors.CodeInternal) {
		t.Errorf("want Internal, got %v", err)
	}
}
