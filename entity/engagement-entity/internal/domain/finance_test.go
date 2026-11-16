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

package domain

import "testing"

func TestSplitInvoiceRejectReason_PurchaseOrderID(t *testing.T) {
	r := "NetSuite PO: 5501"
	inv := FinanceInvoice{Status: InvoiceStatusComplete, RejectReason: &r}
	inv.SplitInvoiceRejectReason()
	if inv.RejectReason != nil || inv.NetSuiteReference == nil || *inv.NetSuiteReference != "5501" || inv.NetSuiteFailed {
		t.Errorf("inv = %+v", inv)
	}
}
