package service

import (
	"context"

	"github.com/hosea3000/work2api/internal/model"
)

// --- codebuddy 仓储 fake ---

type fakeCbRepo struct {
	saved   []model.CodeBuddyCredential
	records []model.CodeBuddyCheckinRecord
}

func (f *fakeCbRepo) List(context.Context) ([]model.CodeBuddyCredential, error) { return f.saved, nil }

func (f *fakeCbRepo) Get(_ context.Context, id string) (*model.CodeBuddyCredential, error) {
	for i := range f.saved {
		if f.saved[i].Id == id {
			return &f.saved[i], nil
		}
	}
	return nil, nil
}

func (f *fakeCbRepo) GetByUserId(_ context.Context, uid string) (*model.CodeBuddyCredential, error) {
	for i := range f.saved {
		if f.saved[i].UserId == uid {
			return &f.saved[i], nil
		}
	}
	return nil, nil
}

func (f *fakeCbRepo) GetByAccountUid(_ context.Context, uid string) (*model.CodeBuddyCredential, error) {
	for i := range f.saved {
		if f.saved[i].AccountUid != nil && *f.saved[i].AccountUid == uid {
			return &f.saved[i], nil
		}
	}
	return nil, nil
}

func (f *fakeCbRepo) Create(_ context.Context, c *model.CodeBuddyCredential) error {
	f.saved = append(f.saved, *c)
	return nil
}

func (f *fakeCbRepo) Update(_ context.Context, c *model.CodeBuddyCredential) error {
	for i := range f.saved {
		if f.saved[i].Id == c.Id {
			f.saved[i] = *c
			return nil
		}
	}
	return nil
}

func (f *fakeCbRepo) Delete(_ context.Context, id string) error {
	for i := range f.saved {
		if f.saved[i].Id == id {
			f.saved = append(f.saved[:i], f.saved[i+1:]...)
			return nil
		}
	}
	return nil
}

func (f *fakeCbRepo) UpdateQuota(_ context.Context, id string, total, remaining float64) error {
	for i := range f.saved {
		if f.saved[i].Id == id {
			f.saved[i].QuotaTotal = &total
			f.saved[i].QuotaRemaining = &remaining
			return nil
		}
	}
	return nil
}

func (f *fakeCbRepo) GetCheckinRecord(_ context.Context, credId, date string) (*model.CodeBuddyCheckinRecord, error) {
	for i := range f.records {
		if f.records[i].CredentialId == credId && f.records[i].CheckinDate == date {
			return &f.records[i], nil
		}
	}
	return nil, nil
}

func (f *fakeCbRepo) SaveCheckinRecord(_ context.Context, r *model.CodeBuddyCheckinRecord) error {
	f.records = append(f.records, *r)
	return nil
}

// --- trae 仓储 fake ---

type fakeTraeRepo struct {
	saved   []model.TraeCredential
	records []model.TraeCheckinRecord
}

func (f *fakeTraeRepo) List(context.Context) ([]model.TraeCredential, error) { return f.saved, nil }

func (f *fakeTraeRepo) Get(_ context.Context, id string) (*model.TraeCredential, error) {
	for i := range f.saved {
		if f.saved[i].Id == id {
			return &f.saved[i], nil
		}
	}
	return nil, nil
}

func (f *fakeTraeRepo) GetByUserId(_ context.Context, uid string) (*model.TraeCredential, error) {
	for i := range f.saved {
		if f.saved[i].UserId == uid {
			return &f.saved[i], nil
		}
	}
	return nil, nil
}

func (f *fakeTraeRepo) Create(_ context.Context, c *model.TraeCredential) error {
	f.saved = append(f.saved, *c)
	return nil
}

func (f *fakeTraeRepo) Update(_ context.Context, c *model.TraeCredential) error {
	for i := range f.saved {
		if f.saved[i].Id == c.Id {
			f.saved[i] = *c
			return nil
		}
	}
	return nil
}

func (f *fakeTraeRepo) Delete(_ context.Context, id string) error {
	for i := range f.saved {
		if f.saved[i].Id == id {
			f.saved = append(f.saved[:i], f.saved[i+1:]...)
			return nil
		}
	}
	return nil
}

func (f *fakeTraeRepo) UpdateQuota(_ context.Context, id string, total, remaining float64) error {
	for i := range f.saved {
		if f.saved[i].Id == id {
			f.saved[i].QuotaTotal = &total
			f.saved[i].QuotaRemaining = &remaining
			return nil
		}
	}
	return nil
}

func (f *fakeTraeRepo) GetCheckinRecord(_ context.Context, credId, date string) (*model.TraeCheckinRecord, error) {
	for i := range f.records {
		if f.records[i].CredentialId == credId && f.records[i].CheckinDate == date {
			return &f.records[i], nil
		}
	}
	return nil, nil
}

func (f *fakeTraeRepo) SaveCheckinRecord(_ context.Context, r *model.TraeCheckinRecord) error {
	f.records = append(f.records, *r)
	return nil
}

// --- pool_state 仓储 fake ---

type fakeStateRepo struct {
	states map[string]*model.PoolState
}

func (f *fakeStateRepo) Get(_ context.Context, provider string) (*model.PoolState, error) {
	if f.states == nil {
		return nil, nil
	}
	return f.states[provider], nil
}

func (f *fakeStateRepo) Upsert(_ context.Context, s *model.PoolState) error {
	if f.states == nil {
		f.states = map[string]*model.PoolState{}
	}
	cp := *s
	f.states[s.Provider] = &cp
	return nil
}

func newCbCred(id, status string) model.CodeBuddyCredential {
	return model.CodeBuddyCredential{Id: id, Status: status, BearerToken: "at-" + id, UserId: "u-" + id}
}

func newTraeCred(id, status string) model.TraeCredential {
	return model.TraeCredential{Id: id, Status: status, BearerToken: "at-" + id, UserId: "u-" + id}
}
