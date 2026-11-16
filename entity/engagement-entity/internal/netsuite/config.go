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

// Package netsuite ports the legacy NetSuite RESTlet client (OAuth 1.0a TBA,
// HMAC-SHA256): purchase orders, vendor bills, marketing events and lookups.
package netsuite

import (
	"strings"

	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/config"
)

// Script is a RESTlet deployment (legacy ScriptAndDeploymentId). The
// deployment defaults to 1, as in the legacy record.
type Script struct {
	ID     string
	Deploy string
}

func (s Script) set() bool { return s.ID != "" }

// Config is the NetSuite account and the Travel app's RESTlets.
type Config struct {
	// Enabled switches pushes and lookups on (NETSUITE_PUSH_ENABLED).
	Enabled bool

	AccountID      string
	ConsumerKey    string
	ConsumerSecret string
	TokenID        string
	TokenSecret    string
	// RestletsBaseURL is https://<account>.restlets.api.netsuite.com.
	RestletsBaseURL string

	// CreatePurchaseOrder and GetPurchaseOrder are the legacy
	// createPurchaseOrder and getPurchaseOrder RESTlets (invoices).
	CreatePurchaseOrder Script
	GetPurchaseOrder    Script
	// CreateVendorBill and GetVendorBill are the legacy insertPerDiemPayments
	// and getVendorBill RESTlets (per diem payments).
	CreateVendorBill Script
	GetVendorBill    Script
	// CreateMarketingEvent is the legacy createMarketingEvent RESTlet.
	CreateMarketingEvent Script

	// PurchaseOrderTaxCode is the one tax code sent on every purchase order
	// (legacy [app_travel.operation] taxCode). Vendor bills use the
	// traveller's subsidiary tax code instead.
	PurchaseOrderTaxCode string
}

// ConfigFrom builds the Config from the entity's NetSuite settings.
func ConfigFrom(c config.NetSuiteConfig) Config {
	t := strings.TrimSpace
	deploy := func(v string) string {
		if v = t(v); v != "" {
			return v
		}
		return "1"
	}
	return Config{
		Enabled:              c.PushEnabled,
		AccountID:            c.AccountID,
		ConsumerKey:          c.ConsumerKey,
		ConsumerSecret:       c.ConsumerSecret,
		TokenID:              c.TokenID,
		TokenSecret:          c.TokenSecret,
		RestletsBaseURL:      strings.TrimRight(c.RestletsBaseURL, "/"),
		CreatePurchaseOrder:  Script{ID: t(c.CreatePurchaseOrderScriptID), Deploy: deploy(c.CreatePurchaseOrderDeployID)},
		GetPurchaseOrder:     Script{ID: t(c.GetPurchaseOrderScriptID), Deploy: deploy(c.GetPurchaseOrderDeployID)},
		CreateVendorBill:     Script{ID: t(c.CreateVendorBillScriptID), Deploy: deploy(c.CreateVendorBillDeployID)},
		GetVendorBill:        Script{ID: t(c.GetVendorBillScriptID), Deploy: deploy(c.GetVendorBillDeployID)},
		CreateMarketingEvent: Script{ID: t(c.CreateMarketingEventScriptID), Deploy: deploy(c.CreateMarketingEventDeployID)},
		PurchaseOrderTaxCode: t(c.PurchaseOrderTaxCode),
	}
}

// Missing names the settings an enabled Config lacks; nil when complete.
// config.Validate already refuses to start without them; this guards
// callers that build a Config by hand.
func (c Config) Missing() []string {
	if !c.Enabled {
		return nil
	}
	var out []string
	for _, kv := range []struct {
		key string
		ok  bool
	}{
		{"NETSUITE_ACCOUNT_ID", c.AccountID != ""},
		{"NETSUITE_CONSUMER_KEY", c.ConsumerKey != ""},
		{"NETSUITE_CONSUMER_SECRET", c.ConsumerSecret != ""},
		{"NETSUITE_TOKEN_ID", c.TokenID != ""},
		{"NETSUITE_TOKEN_SECRET", c.TokenSecret != ""},
		{"NETSUITE_RESTLETS_BASE_URL", c.RestletsBaseURL != ""},
		{"NETSUITE_CREATE_PURCHASE_ORDER_SCRIPT_ID", c.CreatePurchaseOrder.set()},
		{"NETSUITE_GET_PURCHASE_ORDER_SCRIPT_ID", c.GetPurchaseOrder.set()},
		{"NETSUITE_CREATE_VENDOR_BILL_SCRIPT_ID", c.CreateVendorBill.set()},
		{"NETSUITE_GET_VENDOR_BILL_SCRIPT_ID", c.GetVendorBill.set()},
		{"NETSUITE_CREATE_MARKETING_EVENT_SCRIPT_ID", c.CreateMarketingEvent.set()},
		{"NETSUITE_PURCHASE_ORDER_TAX_CODE", c.PurchaseOrderTaxCode != ""},
	} {
		if !kv.ok {
			out = append(out, kv.key)
		}
	}
	return out
}
