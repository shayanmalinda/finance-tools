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

package email

import (
	"reflect"
	"testing"
)

func engagementTestData(typeID int) EngagementEmailData {
	return EngagementEmailData{ID: "EVT00509", EngagementTypeID: typeID, RequestStatus: "Pending Approval",
		CustomerName: "WSO2", ProjectName: "WSO2Con <Asia>", Nature: "Physical", StartDate: "2026-11-20T00:00:00Z",
		Requester: "consultant@wso2.com", SubRegion: "LK"}
}

func TestEngagementMessage_CreatedEvent(t *testing.T) {
	r := EngagementRecipients{AppBaseURL: "https://one.wso2.com/", MarketingApprovers: []string{"mkt@wso2.com"},
		InternalApprovers: []string{"int@wso2.com"}, Routing: []string{"allocations@wso2.com"}}
	m := EngagementMessage(EngagementCreated, engagementTestData(5), r, "me@wso2.com")
	if m.Subject != "[Allocations] Approval request for EVT00509 : WSO2 - WSO2Con <Asia>" || m.TemplateID != "marketingEvent" {
		t.Errorf("subject %q template %q", m.Subject, m.TemplateID)
	}
	if !reflect.DeepEqual(m.To, []string{"me@wso2.com", "mkt@wso2.com", "allocations@wso2.com", "me@wso2.com"}) ||
		!reflect.DeepEqual(m.CC, []string{"consultant@wso2.com"}) {
		t.Errorf("to %v cc %v", m.To, m.CC)
	}
	c := m.Content
	if c["title"] != "New Marketing Event created - Please approve" || c["project_name"] != "WSO2Con &lt;Asia&gt;" ||
		c["allocate_now_link"] != "https://one.wso2.com/finance/engagement-app/approvals/marketing" ||
		c["start_date"] != "2026-11-20" || c["global_pod"] != "LK" || c["business_unit"] != "N/A" || c["contracts_signed"] != "No" {
		t.Errorf("content = %v", c)
	}
}

func TestEngagementMessage_OtherEvents(t *testing.T) {
	r := EngagementRecipients{AppBaseURL: "https://one.wso2.com", TravelGroup: "travel@wso2.com", Consultants: []string{"jane@wso2.com"}}
	upd := EngagementMessage(EngagementUpdated, engagementTestData(1), r, "me@wso2.com")
	if upd.Subject != "Re: [Allocations] Approval request for EVT00509 : WSO2 - WSO2Con <Asia>" || upd.Content["title"] != "Internal Engagement Updated" ||
		upd.Content["allocate_now_link"] != "https://one.wso2.com/finance/engagement-app/engagements/EVT00509/allocations" ||
		!reflect.DeepEqual(upd.To, []string{"me@wso2.com"}) {
		t.Errorf("updated = %+v", upd)
	}
	app := EngagementMessage(EngagementUpdatedAndApproved, engagementTestData(5), r, "me@wso2.com")
	if app.Content["title"] != "Marketing Event Updated and Approved" ||
		app.Content["description"] != "Please note that the following marketing event has been updated and approved." ||
		!reflect.DeepEqual(app.CC, []string{"consultant@wso2.com", "travel@wso2.com"}) || app.NotificationType != "ENGAGEMENT_APPROVED" {
		t.Errorf("approved = %+v", app)
	}
	d := engagementTestData(6)
	d.Nature = "Off-site"
	r.TravelAffected = true
	can := EngagementMessage(EngagementCancelled, d, r, "me@wso2.com")
	if can.TemplateID != "customerEngagementCancelled" || can.Subject != "Re: [Allocations] Consultant request for EVT00509 : WSO2 - WSO2Con <Asia>" ||
		!reflect.DeepEqual(can.To, []string{"jane@wso2.com", "me@wso2.com"}) || !reflect.DeepEqual(can.CC, []string{"travel@wso2.com", "consultant@wso2.com"}) {
		t.Errorf("cancelled = %+v", can)
	}
}
