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

package server

import (
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/domain"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/handler"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/service"
)

// registerSearchRoutes registers global search. It reuses the list services
// the other areas built, so a caller finds only what each list shows them;
// a group whose area is not registered is left out. Registered after the
// areas it reads.
func registerSearchRoutes(a *area) {
	src := service.SearchSources{Travel: a.travel, Engagements: a.engagements, Allocations: a.allocations}
	if a.finance != nil {
		src.Invoices, src.Payments = a.finance, a.finance
	}
	// Gate: any privilege of a searchable group (the union of their list gates).
	gate := domain.Gate{
		domain.AppTravel:      {1, 2, 18, 32, 33, 34, 39, 40, 41},
		domain.AppAllocations: {2, 3, 9, 11, 12, 13, 19},
	}
	a.handle("GET /search", gate, handler.NewSearchHandler(service.NewSearchService(src)).Search)
}
