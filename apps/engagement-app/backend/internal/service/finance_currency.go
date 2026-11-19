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
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/apierror"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/rest"
)

// FinanceCurrencyConverter is the legacy currency conversion service
// (digiops-finance operations/currency-convert-service, Choreo component
// finance/currency-convert-service; legacy config [app_travel.utils]
// currencyConvertServiceEndpoint with its own OAuth2 client credentials). It
// wraps NetSuite's currency rate RESTlet.
type FinanceCurrencyConverter interface {
	// Rates returns base's exchange rates on date (YYYY-MM-DD), keyed by
	// currency code (POST /convert-currency {baseCode, date}).
	Rates(ctx context.Context, base, date string) (map[string]float64, error)
}

type financeCurrencyClient struct{ rest *rest.Client }

// NewFinanceCurrencyClient returns the converter at baseURL; httpClient
// carries the OAuth2 client credentials (rest.NewOAuthHTTPClient).
func NewFinanceCurrencyClient(baseURL string, httpClient *http.Client) FinanceCurrencyConverter {
	return &financeCurrencyClient{rest: rest.New("currency-convert-service", baseURL, httpClient)}
}

// financeRate accepts the rate as a JSON number or string ("200.00").
type financeRate float64

func (r *financeRate) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return fmt.Errorf("exchange rate %s: %w", string(b), err)
	}
	*r = financeRate(f)
	return nil
}

func (c *financeCurrencyClient) Rates(ctx context.Context, base, date string) (map[string]float64, error) {
	var resp []struct {
		CurrencyCode string      `json:"currencyCode"`
		ExchangeRate financeRate `json:"exchangeRate"`
	}
	if err := c.rest.Post(ctx, "/convert-currency", map[string]string{"baseCode": base, "date": date}, &resp); err != nil {
		return nil, err
	}
	out := make(map[string]float64, len(resp))
	for _, r := range resp {
		out[r.CurrencyCode] = float64(r.ExchangeRate)
	}
	return out, nil
}

// financeFX converts amounts for one request, caching each (base, date).
type financeFX struct {
	conv  FinanceCurrencyConverter
	rates map[string]map[string]float64
}

func newFinanceFX(conv FinanceCurrencyConverter) *financeFX {
	return &financeFX{conv: conv, rates: map[string]map[string]float64{}}
}

// errFinanceNoConverter is answered 503.
var errFinanceNoConverter = &apierror.ServiceUnavailableError{Msg: "Currency conversion is not configured."}

// convert ports the legacy convertCurrency: the same currency is returned
// as is; otherwise amount / rate[target] of the rates with baseCode = source
// on the date part of dateTime (the legacy divides, it does not multiply).
func (fx *financeFX) convert(ctx context.Context, src, tgt string, amount float64, dateTime string) (float64, error) {
	if strings.EqualFold(src, tgt) {
		return amount, nil
	}
	if fx.conv == nil {
		return 0, errFinanceNoConverter
	}
	date := dateTime
	if len(date) > 10 {
		date = date[:10]
	}
	key := strings.ToUpper(src) + "|" + date
	rates, ok := fx.rates[key]
	if !ok {
		r, err := fx.conv.Rates(ctx, strings.ToUpper(src), date)
		if err != nil {
			return 0, err
		}
		fx.rates[key] = r
		rates = r
	}
	rate, ok := rates[strings.ToUpper(tgt)]
	if !ok || rate <= 0 || math.IsNaN(rate) || math.IsInf(rate, 0) {
		return 0, &apierror.ServiceUnavailableError{Msg: fmt.Sprintf("Error while currency conversion: no %s rate for %s on %s", tgt, src, date)}
	}
	v := amount / rate
	if math.IsInf(v, 0) || math.IsNaN(v) {
		return 0, &apierror.ServiceUnavailableError{Msg: fmt.Sprintf("Error while currency conversion: the %s rate for %s on %s is out of range", tgt, src, date)}
	}
	return v, nil
}
