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
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/cache"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/hrentity"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/domain"
)

const (
	// travelPeopleTTL keeps an HR answer (name, location, company) this long.
	travelPeopleTTL = 10 * time.Minute
	// travelPeopleParallel bounds concurrent HR lookups for one page.
	travelPeopleParallel = 8
)

// travelPerson is what the travel screens and emails need from HR.
type travelPerson struct {
	Name     *string
	Location *string
	Company  *string
}

// travelPeople looks travellers up in HR (legacy getEmployeeData: name,
// location as the base location, company for the finance emails), with a
// short cache. A failed lookup is logged and leaves the fields empty: unlike
// the legacy app (500 when HR had no such employee), a travel request still
// opens.
type travelPeople struct {
	hr    hrentity.Client
	cache *cache.Cache[string, travelPerson]
}

func newTravelPeople(hr hrentity.Client) *travelPeople {
	return &travelPeople{hr: hr, cache: cache.New[string, travelPerson](travelPeopleTTL)}
}

func (tp *travelPeople) get(ctx context.Context, email string) travelPerson {
	key := strings.ToLower(strings.TrimSpace(email))
	if key == "" {
		return travelPerson{}
	}
	if p, ok := tp.cache.Get(key); ok {
		return p
	}
	e, err := tp.hr.Employee(ctx, key)
	if err != nil {
		slog.WarnContext(ctx, "travel: HR lookup failed", "err", err)
		return travelPerson{}
	}
	var p travelPerson
	if e != nil {
		if name := strings.TrimSpace(e.FirstName + " " + e.LastName); name != "" {
			p.Name = &name
		}
		p.Location, p.Company = e.Location, e.Company
	}
	tp.cache.Set(key, p)
	return p
}

// many looks up every distinct email, a few at a time.
func (tp *travelPeople) many(ctx context.Context, emails []string) map[string]travelPerson {
	out := map[string]travelPerson{}
	var todo []string
	for _, e := range emails {
		k := strings.ToLower(strings.TrimSpace(e))
		if _, seen := out[k]; k == "" || seen {
			continue
		}
		out[k] = travelPerson{}
		todo = append(todo, k)
	}
	var (
		mu  sync.Mutex
		wg  sync.WaitGroup
		sem = make(chan struct{}, travelPeopleParallel)
	)
	for _, k := range todo {
		wg.Add(1)
		sem <- struct{}{}
		go func(k string) {
			defer wg.Done()
			defer func() { <-sem }()
			p := tp.get(ctx, k)
			mu.Lock()
			out[k] = p
			mu.Unlock()
		}(k)
	}
	wg.Wait()
	return out
}

func (tp *travelPeople) of(m map[string]travelPerson, email string) travelPerson {
	return m[strings.ToLower(strings.TrimSpace(email))]
}

// fillNames sets the HR name of every person that has none, with one lookup
// per distinct email; a failed lookup leaves the email only.
func (tp *travelPeople) fillNames(ctx context.Context, ps ...*domain.Person) {
	if tp == nil {
		return
	}
	var emails []string
	for _, p := range ps {
		if p != nil && p.Name == nil {
			emails = append(emails, p.Email)
		}
	}
	if len(emails) == 0 {
		return
	}
	people := tp.many(ctx, emails)
	for _, p := range ps {
		if p != nil && p.Name == nil {
			p.Name = tp.of(people, p.Email).Name
		}
	}
}
