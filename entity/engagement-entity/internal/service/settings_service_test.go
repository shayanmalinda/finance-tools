// Copyright (c) 2026 WSO2 LLC. (https://www.wso2.com).
//
// WSO2 LLC. licenses this file to you under the Apache License,
// Version 2.0 (the "License"); you may not use this file except
// in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied. See the License for the
// specific language governing permissions and limitations
// under the License.

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/apierror"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/domain"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/repository"
)

type settingsFakeRepo struct {
	repository.SettingsRepository
	taken   bool
	saved   domain.SettingsAllocationTypeInput
	missing bool
}

func (f *settingsFakeRepo) EngagementTypeExists(_ context.Context, id int) (bool, error) {
	return id < 8, nil
}
func (f *settingsFakeRepo) AllocationTypeNameTaken(context.Context, int, string, int) (bool, error) {
	return f.taken, nil
}
func (f *settingsFakeRepo) CreateAllocationType(_ context.Context, in domain.SettingsAllocationTypeInput) (int, error) {
	f.saved = in
	return 5, nil
}
func (f *settingsFakeRepo) UpdateAllocationType(_ context.Context, _ int, in domain.SettingsAllocationTypeInput) error {
	f.saved = in
	if f.missing {
		return repository.ErrSettingsAllocationTypeNotFound
	}
	return nil
}
func (f *settingsFakeRepo) DeleteAllocationType(context.Context, int, string) error {
	if f.missing {
		return repository.ErrSettingsAllocationTypeNotFound
	}
	return nil
}
func (f *settingsFakeRepo) AllocationType(_ context.Context, id int) (domain.AllocationType, error) {
	if f.missing {
		return domain.AllocationType{}, repository.ErrSettingsAllocationTypeNotFound
	}
	return domain.AllocationType{ID: id}, nil
}

func TestSettingsAllocationTypes(t *testing.T) {
	repo := &settingsFakeRepo{}
	svc := NewSettingsService(repo)
	in := domain.SettingsAllocationTypeInput{EngagementTypeID: 1, AllocationTypeName: " Digital ", DefaultEngagementCode: "DIG-GEN",
		Department: "Digital ops", EmailToAllocations: "No", Color: "#00aabb", Actor: "me"}
	at, err := svc.CreateAllocationType(context.Background(), in)
	if err != nil || at.ID != 5 || repo.saved.Department != "DIGITAL OPS" || repo.saved.AllocationTypeName != "Digital" {
		t.Fatalf("at %+v saved %+v err %v", at, repo.saved, err)
	}
	var ve *apierror.ValidationError
	for _, mut := range []func(*domain.SettingsAllocationTypeInput){
		func(i *domain.SettingsAllocationTypeInput) { i.AllocationTypeName = "" },
		func(i *domain.SettingsAllocationTypeInput) { i.EngagementTypeID = 9 },
		func(i *domain.SettingsAllocationTypeInput) { i.EmailToAllocations = "Maybe" },
		func(i *domain.SettingsAllocationTypeInput) { i.Color = "red" },
		func(i *domain.SettingsAllocationTypeInput) { i.Actor = "" },
	} {
		bad := in
		mut(&bad)
		if _, err := svc.CreateAllocationType(context.Background(), bad); !errors.As(err, &ve) {
			t.Fatalf("bad %+v: err %v", bad, err)
		}
	}
	var ce *apierror.ConflictError
	if _, err := NewSettingsService(&settingsFakeRepo{taken: true}).CreateAllocationType(context.Background(), in); !errors.As(err, &ce) {
		t.Fatalf("duplicate: %v", err)
	}
	var nf *apierror.NotFoundError
	if _, err := NewSettingsService(&settingsFakeRepo{missing: true, taken: true}).UpdateAllocationType(context.Background(), 3, in); !errors.As(err, &nf) {
		t.Fatalf("update missing: %v", err)
	}
	if err := NewSettingsService(&settingsFakeRepo{missing: true}).DeleteAllocationType(context.Background(), 3, "me"); !errors.As(err, &nf) {
		t.Fatalf("delete missing: %v", err)
	}
}
