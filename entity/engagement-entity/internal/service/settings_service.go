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
	"fmt"
	"regexp"
	"strings"

	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/apierror"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/domain"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/repository"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/textnorm"
)

// SettingsService manages allocation types (Settings → Allocation Types).
type SettingsService interface {
	CreateAllocationType(ctx context.Context, in domain.SettingsAllocationTypeInput) (domain.AllocationType, error)
	UpdateAllocationType(ctx context.Context, id int, in domain.SettingsAllocationTypeInput) (domain.AllocationType, error)
	DeleteAllocationType(ctx context.Context, id int, actor string) error
}

type settingsService struct{ repo repository.SettingsRepository }

// NewSettingsService constructs a SettingsService.
func NewSettingsService(repo repository.SettingsRepository) SettingsService {
	return &settingsService{repo: repo}
}

var settingsColorPattern = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

// settingsNormalise applies the legacy form rules: the three required
// fields, 90-character limits, Notify To Allocations Yes / No / empty, the
// department stored upper case.
func (s *settingsService) settingsNormalise(ctx context.Context, in *domain.SettingsAllocationTypeInput) error {
	in.AllocationTypeName = strings.TrimSpace(in.AllocationTypeName)
	in.DefaultEngagementCode = strings.TrimSpace(in.DefaultEngagementCode)
	in.EngagementCodeSuffix = strings.TrimSpace(in.EngagementCodeSuffix)
	in.Department = strings.ToUpper(strings.TrimSpace(in.Department))
	in.EmailToAllocations = strings.TrimSpace(in.EmailToAllocations)
	in.Color = strings.TrimSpace(in.Color)
	if in.EngagementTypeID <= 0 || in.AllocationTypeName == "" || in.DefaultEngagementCode == "" {
		return &apierror.ValidationError{Msg: "Engagement Type, Allocation Type and Default Engagement Code are required."}
	}
	for _, v := range []string{in.AllocationTypeName, in.DefaultEngagementCode, in.EngagementCodeSuffix, in.Department} {
		if len(v) > 90 {
			return &apierror.ValidationError{Msg: "Allocation Type, Default Engagement Code, Engagement Code Suffix and Department must be at most 90 characters."}
		}
	}
	if in.EmailToAllocations != "" && in.EmailToAllocations != "Yes" && in.EmailToAllocations != "No" {
		return &apierror.ValidationError{Msg: "Notify To Allocations must be Yes or No."}
	}
	if in.Color != "" && !settingsColorPattern.MatchString(in.Color) {
		return &apierror.ValidationError{Msg: "Color must be a hex colour such as #00aabb."}
	}
	if strings.TrimSpace(in.Actor) == "" {
		return &apierror.ValidationError{Msg: "actor is required"}
	}
	ok, err := s.repo.EngagementTypeExists(ctx, in.EngagementTypeID)
	if err != nil {
		return err
	}
	if !ok {
		return &apierror.ValidationError{Msg: "Select the engagement type."}
	}
	in.AllocationTypeName = textnorm.Latin1(in.AllocationTypeName)
	in.DefaultEngagementCode = textnorm.Latin1(in.DefaultEngagementCode)
	in.EngagementCodeSuffix = textnorm.Latin1(in.EngagementCodeSuffix)
	in.Department = textnorm.Latin1(in.Department)
	return nil
}

func (s *settingsService) checkName(ctx context.Context, in domain.SettingsAllocationTypeInput, exceptID int) error {
	taken, err := s.repo.AllocationTypeNameTaken(ctx, in.EngagementTypeID, in.AllocationTypeName, exceptID)
	if err != nil {
		return err
	}
	if taken {
		return &apierror.ConflictError{Msg: "An allocation type with this name already exists for the engagement type."}
	}
	return nil
}

func (s *settingsService) CreateAllocationType(ctx context.Context, in domain.SettingsAllocationTypeInput) (domain.AllocationType, error) {
	if err := s.settingsNormalise(ctx, &in); err != nil {
		return domain.AllocationType{}, err
	}
	if err := s.checkName(ctx, in, 0); err != nil {
		return domain.AllocationType{}, err
	}
	id, err := s.repo.CreateAllocationType(ctx, in)
	if err != nil {
		return domain.AllocationType{}, err
	}
	return s.repo.AllocationType(ctx, id)
}

func settingsNotFound(id int) error {
	return &apierror.NotFoundError{Msg: fmt.Sprintf("Allocation type %d not found.", id)}
}

func (s *settingsService) UpdateAllocationType(ctx context.Context, id int, in domain.SettingsAllocationTypeInput) (domain.AllocationType, error) {
	if err := s.settingsNormalise(ctx, &in); err != nil {
		return domain.AllocationType{}, err
	}
	// A missing id is 404 before the name check.
	cur, err := s.repo.AllocationType(ctx, id)
	if errors.Is(err, repository.ErrSettingsAllocationTypeNotFound) || (err == nil && cur.Status == "Deleted") {
		return domain.AllocationType{}, settingsNotFound(id)
	}
	if err != nil {
		return domain.AllocationType{}, err
	}
	if err := s.checkName(ctx, in, id); err != nil {
		return domain.AllocationType{}, err
	}
	if err := s.repo.UpdateAllocationType(ctx, id, in); err != nil {
		if errors.Is(err, repository.ErrSettingsAllocationTypeNotFound) {
			return domain.AllocationType{}, settingsNotFound(id)
		}
		return domain.AllocationType{}, err
	}
	return s.repo.AllocationType(ctx, id)
}

func (s *settingsService) DeleteAllocationType(ctx context.Context, id int, actor string) error {
	if strings.TrimSpace(actor) == "" {
		return &apierror.ValidationError{Msg: "actor is required"}
	}
	if err := s.repo.DeleteAllocationType(ctx, id, actor); err != nil {
		if errors.Is(err, repository.ErrSettingsAllocationTypeNotFound) {
			return settingsNotFound(id)
		}
		return err
	}
	return nil
}
