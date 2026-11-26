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
	"strings"
	"testing"
)

func engagementTestData(typeID int) EngagementEmailData {
	return EngagementEmailData{ID: "EVT00509", EngagementTypeID: typeID, RequestStatus: "Pending Approval",
		CustomerName: "WSO2", ProjectName: "WSO2Con <Asia>", Nature: "Physical", StartDate: "2026-11-20T00:00:00Z",
		Requester: "consultant@wso2.com", SubRegion: "LK"}
}

// A new marketing event goes To its approvers ("Approval Needed", button to the
// engagement's details where Approve is); everyone else is copied.
func TestEngagementMessage_CreatedEvent(t *testing.T) {
	r := EngagementRecipients{AppBaseURL: "https://one.wso2.com/", MarketingApprovers: []string{"mkt@wso2.com"},
		InternalApprovers: []string{"int@wso2.com"}, Routing: []string{"allocations@wso2.com"}, RoutingName: "Allocations Team"}
	m := EngagementMessage(EngagementCreated, engagementTestData(5), r, "me@wso2.com")
	if m.Subject != "[Engagement App] Approval Needed: EVT00509 WSO2Con <Asia>" || m.TemplateID != "marketingEvent" {
		t.Errorf("subject %q template %q", m.Subject, m.TemplateID)
	}
	if !reflect.DeepEqual(m.To, []string{"mkt@wso2.com"}) ||
		!reflect.DeepEqual(m.CC, []string{"allocations@wso2.com", "me@wso2.com", "consultant@wso2.com"}) {
		t.Errorf("to %v cc %v", m.To, m.CC)
	}
	c := m.Content
	if c["title"] != "Approval Needed" || c["greeting_name"] != "Marketing Approvers" || c["action_label"] != "Review and Approve" ||
		c["action_link"] != "https://one.wso2.com/finance/engagement-app/engagements/EVT00509/details" {
		t.Errorf("content = %v", c)
	}
	one := r
	one.ApproverName = "Kasun"
	if m := EngagementMessage(EngagementCreated, engagementTestData(5), one, "me@wso2.com"); m.Content["greeting_name"] != "Kasun" {
		t.Errorf("single approver greeting = %q", m.Content["greeting_name"])
	}
	page := renderMessage(t, m)
	for _, want := range []string{"Hi Marketing Approvers,", "WSO2Con &lt;Asia&gt;", "20 Nov 2026", ">Review and Approve</a>"} {
		if !strings.Contains(page, want) {
			t.Errorf("page lacks %q", want)
		}
	}
	if strings.Contains(page, "N/A") {
		t.Error("an empty row shows N/A instead of being hidden")
	}
	// The button comes after the details.
	if strings.Index(page, ">Review and Approve</a>") < strings.Index(page, ">Marketing Event<") {
		t.Error("button is above the details")
	}
}

// Approval and allocation are separate emails: approving sends "Ready to
// Allocate" To whoever allocates, with the approver, creator and travel copied;
// a customer engagement (no approval) gets only that email.
func TestEngagementMessage_ReadyToAllocate(t *testing.T) {
	r := EngagementRecipients{AppBaseURL: "https://one.wso2.com", TravelGroup: "travel@wso2.com",
		Routing: []string{"allocations@wso2.com"}, RoutingName: "Allocations Team"}
	app := EngagementMessage(EngagementUpdatedAndApproved, engagementTestData(5), r, "me@wso2.com")
	if app.Subject != "[Engagement App] Ready to Allocate: EVT00509 WSO2Con <Asia>" || app.Content["title"] != "Ready to Allocate" ||
		app.Content["greeting_name"] != "Allocations Team" || app.Content["action_label"] != "Allocate Now" ||
		app.Content["action_link"] != "https://one.wso2.com/finance/engagement-app/engagements/EVT00509/allocate" ||
		!reflect.DeepEqual(app.To, []string{"allocations@wso2.com"}) ||
		!reflect.DeepEqual(app.CC, []string{"travel@wso2.com", "me@wso2.com", "consultant@wso2.com"}) || app.NotificationType != "ENGAGEMENT_APPROVED" {
		t.Errorf("approved = %+v", app)
	}
	cust := EngagementMessage(EngagementCreated, engagementTestData(3), r, "me@wso2.com")
	if cust.Content["title"] != "Ready to Allocate" || cust.TemplateID != "customerEngagement" ||
		!reflect.DeepEqual(cust.To, []string{"allocations@wso2.com"}) {
		t.Errorf("customer created = %+v", cust)
	}
	// Nobody allocates: the person who made the change gets it, by first name.
	r.Routing, r.ActorName = nil, "Nimal"
	if m := EngagementMessage(EngagementCreated, engagementTestData(3), r, "me@wso2.com"); !reflect.DeepEqual(m.To, []string{"me@wso2.com"}) || m.Content["greeting_name"] != "Nimal" {
		t.Errorf("fallback = %+v", m)
	}
}

func TestEngagementMessage_UpdatedAndCancelled(t *testing.T) {
	r := EngagementRecipients{AppBaseURL: "https://one.wso2.com", TravelGroup: "travel@wso2.com", Consultants: []string{"jane@wso2.com"},
		ConsultantName: "Jane", Routing: []string{"lead@wso2.com"}, RoutingName: "Priya"}
	upd := EngagementMessage(EngagementUpdated, engagementTestData(1), r, "me@wso2.com")
	if upd.Subject != "[Engagement App] Updated: EVT00509 WSO2Con <Asia>" || upd.Content["greeting_name"] != "Priya" ||
		upd.Content["action_link"] != "https://one.wso2.com/finance/engagement-app/engagements/EVT00509/details" ||
		!reflect.DeepEqual(upd.To, []string{"lead@wso2.com"}) {
		t.Errorf("updated = %+v", upd)
	}
	d := engagementTestData(6)
	d.Nature = "Off-site"
	r.TravelAffected = true
	can := EngagementMessage(EngagementCancelled, d, r, "me@wso2.com")
	if can.TemplateID != "customerEngagementCancelled" || can.Subject != "[Engagement App] Cancelled: EVT00509 WSO2Con <Asia>" ||
		can.Content["greeting_name"] != "Jane" || !reflect.DeepEqual(can.To, []string{"jane@wso2.com"}) ||
		!reflect.DeepEqual(can.CC, []string{"lead@wso2.com", "travel@wso2.com", "me@wso2.com", "consultant@wso2.com"}) {
		t.Errorf("cancelled = %+v", can)
	}
}
