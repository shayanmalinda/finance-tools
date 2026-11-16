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

package handler

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/apierror"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/domain"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/filestore"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/service"
)

// travelMaxForm bounds a whole multipart form: the image (5 MB) plus the
// text fields and multipart overhead.
const travelMaxForm = filestore.MaxImageBytes + 1<<20

const travelMaxSearch = 200

// TravelHandler serves travel requests, visas and travel documents.
type TravelHandler struct {
	requests  service.TravelService
	documents service.TravelDocumentsService
}

// NewTravelHandler constructs a TravelHandler.
func NewTravelHandler(requests service.TravelService, documents service.TravelDocumentsService) *TravelHandler {
	return &TravelHandler{requests: requests, documents: documents}
}

func travelSearch(r *http.Request) (string, error) {
	s := strings.TrimSpace(r.URL.Query().Get("search"))
	if utf8.RuneCountInString(s) > travelMaxSearch {
		return "", &apierror.ValidationError{Msg: "search must be at most 200 characters."}
	}
	return s, nil
}

func travelListQuery(r *http.Request) (domain.TravelListQuery, error) {
	q := domain.TravelListQuery{
		Segment: strings.TrimSpace(r.URL.Query().Get("segment")), Travellers: queryList(r, "traveller"),
		Countries: queryList(r, "country"), Agencies: queryList(r, "agency"), Statuses: queryList(r, "status"),
	}
	var err error
	if q.Search, err = travelSearch(r); err != nil {
		return q, err
	}
	if q.StartFrom, err = queryDate(r, "startFrom"); err != nil {
		return q, err
	}
	if q.EndTo, err = queryDate(r, "endTo"); err != nil {
		return q, err
	}
	q.Limit, q.Offset, err = pagination(r)
	return q, err
}

func travelRecordQuery(r *http.Request) (domain.TravelRecordQuery, error) {
	q := domain.TravelRecordQuery{
		Travellers: queryList(r, "traveller"), Countries: queryList(r, "country"),
		DocumentTypes: queryList(r, "documentType"), Status: strings.TrimSpace(r.URL.Query().Get("status")),
	}
	var err error
	if q.Search, err = travelSearch(r); err != nil {
		return q, err
	}
	q.Limit, q.Offset, err = pagination(r)
	return q, err
}

func travelPathID(r *http.Request, name string) (int, error) {
	id, err := strconv.Atoi(r.PathValue(name))
	if err != nil || id <= 0 {
		return 0, &apierror.ValidationError{Msg: name + " must be a positive whole number."}
	}
	return id, nil
}

// ── Travel requests ───────────────────────────────────────────────────────

// ListTravelRequests handles GET /travel-requests.
func (h *TravelHandler) ListTravelRequests(w http.ResponseWriter, r *http.Request) {
	p, ok := permsFrom(w, r)
	if !ok {
		return
	}
	q, err := travelListQuery(r)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	resp, err := h.requests.List(r.Context(), p, q)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}

// ListMyTravelRequests handles GET /me/travel-requests.
func (h *TravelHandler) ListMyTravelRequests(w http.ResponseWriter, r *http.Request) {
	p, ok := permsFrom(w, r)
	if !ok {
		return
	}
	q, err := travelListQuery(r)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	resp, err := h.requests.ListMine(r.Context(), p, q)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}

// GetTravelRequest handles GET /travel-requests/{jobNumber}.
func (h *TravelHandler) GetTravelRequest(w http.ResponseWriter, r *http.Request) {
	p, ok := permsFrom(w, r)
	if !ok {
		return
	}
	resp, err := h.requests.Get(r.Context(), p, r.PathValue("jobNumber"))
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}

// UpdateTravelRequest handles PATCH /travel-requests/{jobNumber}.
func (h *TravelHandler) UpdateTravelRequest(w http.ResponseWriter, r *http.Request) {
	p, ok := permsFrom(w, r)
	if !ok {
		return
	}
	var in domain.TravelRequestUpdate
	if !decodeJSON(w, r, &in) {
		return
	}
	resp, err := h.requests.Update(r.Context(), p, r.PathValue("jobNumber"), in)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}

// VisaOptions handles GET /travel-requests/{jobNumber}/visa-options.
func (h *TravelHandler) VisaOptions(w http.ResponseWriter, r *http.Request) {
	p, ok := permsFrom(w, r)
	if !ok {
		return
	}
	resp, err := h.requests.VisaOptions(r.Context(), p, r.PathValue("jobNumber"))
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}

// ── Multipart forms ───────────────────────────────────────────────────────

var errTravelTooLarge = errors.New("too large")

// travelParseForm parses a multipart form of at most travelMaxForm bytes
// and reads its "image" part. Image bytes and form values are never logged.
func travelParseForm(w http.ResponseWriter, r *http.Request) (domain.TravelImage, error) {
	if !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "multipart/form-data") {
		return domain.TravelImage{}, &apierror.ValidationError{Msg: "The request must be multipart/form-data."}
	}
	r.Body = http.MaxBytesReader(w, r.Body, travelMaxForm)
	if err := r.ParseMultipartForm(travelMaxForm); err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			return domain.TravelImage{}, errTravelTooLarge
		}
		return domain.TravelImage{}, &apierror.ValidationError{Msg: "The form could not be read."}
	}
	file, _, err := r.FormFile("image")
	if errors.Is(err, http.ErrMissingFile) {
		return domain.TravelImage{}, nil
	}
	if err != nil {
		return domain.TravelImage{}, &apierror.ValidationError{Msg: "The image could not be read."}
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, filestore.MaxImageBytes+1))
	if err != nil {
		return domain.TravelImage{}, &apierror.ValidationError{Msg: "The image could not be read."}
	}
	if len(data) > filestore.MaxImageBytes {
		return domain.TravelImage{}, errTravelTooLarge
	}
	if len(data) == 0 {
		return domain.TravelImage{}, nil
	}
	return domain.TravelImage{Data: data, Present: true}, nil
}

func travelFormError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, errTravelTooLarge) {
		apierror.WriteJSON(w, http.StatusRequestEntityTooLarge, "The image must be 5 MB or smaller.")
		return
	}
	writeServiceError(w, r, err)
}

func travelVisaForm(w http.ResponseWriter, r *http.Request) (domain.TravelVisaForm, error) {
	img, err := travelParseForm(w, r)
	if err != nil {
		return domain.TravelVisaForm{}, err
	}
	f := domain.TravelVisaForm{
		TravellerEmail: r.FormValue("travellerEmail"), Country: r.FormValue("country"),
		VisaType: r.FormValue("visaType"), EntryType: r.FormValue("entryType"),
		IssuedDate: r.FormValue("issuedDate"), StartDate: r.FormValue("startDate"),
		EndDate: r.FormValue("endDate"), Status: r.FormValue("status"), Image: img,
	}
	if v := strings.TrimSpace(r.FormValue("passportId")); v != "" {
		id, err := strconv.Atoi(v)
		if err != nil || id <= 0 {
			return f, &apierror.ValidationError{Msg: "passportId must be a positive whole number."}
		}
		f.PassportID = &id
	}
	return f, nil
}

func travelDocumentForm(w http.ResponseWriter, r *http.Request) (domain.TravelDocumentForm, error) {
	img, err := travelParseForm(w, r)
	if err != nil {
		return domain.TravelDocumentForm{}, err
	}
	return domain.TravelDocumentForm{
		TravellerEmail: r.FormValue("travellerEmail"), DocumentType: r.FormValue("documentType"),
		DocumentNumber: r.FormValue("documentNumber"), Country: r.FormValue("country"),
		IssuedDate: r.FormValue("issuedDate"), ExpiryDate: r.FormValue("expiryDate"),
		Status: r.FormValue("status"), Image: img,
	}, nil
}

func travelWriteImage(w http.ResponseWriter, contentType string, data []byte) {
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Header().Set("Content-Disposition", "inline")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// ── Visas ─────────────────────────────────────────────────────────────────

func (h *TravelHandler) listVisas(w http.ResponseWriter, r *http.Request, own bool) {
	p, ok := permsFrom(w, r)
	if !ok {
		return
	}
	q, err := travelRecordQuery(r)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	resp, err := h.documents.ListVisas(r.Context(), p, q, own)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}

// ListVisas handles GET /visas.
func (h *TravelHandler) ListVisas(w http.ResponseWriter, r *http.Request) { h.listVisas(w, r, false) }

// ListMyVisas handles GET /me/visas.
func (h *TravelHandler) ListMyVisas(w http.ResponseWriter, r *http.Request) { h.listVisas(w, r, true) }

func (h *TravelHandler) addVisa(w http.ResponseWriter, r *http.Request, own bool) {
	p, ok := permsFrom(w, r)
	if !ok {
		return
	}
	f, err := travelVisaForm(w, r)
	if err != nil {
		travelFormError(w, r, err)
		return
	}
	resp, err := h.documents.AddVisa(r.Context(), p, f, own)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSONStatus(w, http.StatusCreated, resp)
}

// AddVisa handles POST /visas.
func (h *TravelHandler) AddVisa(w http.ResponseWriter, r *http.Request) { h.addVisa(w, r, false) }

// AddMyVisa handles POST /me/visas.
func (h *TravelHandler) AddMyVisa(w http.ResponseWriter, r *http.Request) { h.addVisa(w, r, true) }

// GetVisa handles GET /visas/{visaId}.
func (h *TravelHandler) GetVisa(w http.ResponseWriter, r *http.Request) {
	p, ok := permsFrom(w, r)
	if !ok {
		return
	}
	id, err := travelPathID(r, "visaId")
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	resp, err := h.documents.GetVisa(r.Context(), p, id)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}

// UpdateVisa handles PUT /visas/{visaId}.
func (h *TravelHandler) UpdateVisa(w http.ResponseWriter, r *http.Request) {
	p, ok := permsFrom(w, r)
	if !ok {
		return
	}
	id, err := travelPathID(r, "visaId")
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	f, err := travelVisaForm(w, r)
	if err != nil {
		travelFormError(w, r, err)
		return
	}
	resp, err := h.documents.UpdateVisa(r.Context(), p, id, f)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}

// VisaImage handles GET /visas/{visaId}/image.
func (h *TravelHandler) VisaImage(w http.ResponseWriter, r *http.Request) {
	p, ok := permsFrom(w, r)
	if !ok {
		return
	}
	id, err := travelPathID(r, "visaId")
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	ct, data, err := h.documents.VisaImage(r.Context(), p, id)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	travelWriteImage(w, ct, data)
}

// ── Travel documents ──────────────────────────────────────────────────────

func (h *TravelHandler) listDocuments(w http.ResponseWriter, r *http.Request, own bool) {
	p, ok := permsFrom(w, r)
	if !ok {
		return
	}
	q, err := travelRecordQuery(r)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	resp, err := h.documents.ListDocuments(r.Context(), p, q, own)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}

// ListTravelDocuments handles GET /travel-documents.
func (h *TravelHandler) ListTravelDocuments(w http.ResponseWriter, r *http.Request) {
	h.listDocuments(w, r, false)
}

// ListMyTravelDocuments handles GET /me/travel-documents.
func (h *TravelHandler) ListMyTravelDocuments(w http.ResponseWriter, r *http.Request) {
	h.listDocuments(w, r, true)
}

func (h *TravelHandler) addDocument(w http.ResponseWriter, r *http.Request, own bool) {
	p, ok := permsFrom(w, r)
	if !ok {
		return
	}
	f, err := travelDocumentForm(w, r)
	if err != nil {
		travelFormError(w, r, err)
		return
	}
	resp, err := h.documents.AddDocument(r.Context(), p, f, own)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSONStatus(w, http.StatusCreated, resp)
}

// AddTravelDocument handles POST /travel-documents.
func (h *TravelHandler) AddTravelDocument(w http.ResponseWriter, r *http.Request) {
	h.addDocument(w, r, false)
}

// AddMyTravelDocument handles POST /me/travel-documents.
func (h *TravelHandler) AddMyTravelDocument(w http.ResponseWriter, r *http.Request) {
	h.addDocument(w, r, true)
}

// GetTravelDocument handles GET /travel-documents/{documentId}.
func (h *TravelHandler) GetTravelDocument(w http.ResponseWriter, r *http.Request) {
	p, ok := permsFrom(w, r)
	if !ok {
		return
	}
	id, err := travelPathID(r, "documentId")
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	resp, err := h.documents.GetDocument(r.Context(), p, id)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}

// UpdateTravelDocument handles PUT /travel-documents/{documentId}.
func (h *TravelHandler) UpdateTravelDocument(w http.ResponseWriter, r *http.Request) {
	p, ok := permsFrom(w, r)
	if !ok {
		return
	}
	id, err := travelPathID(r, "documentId")
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	f, err := travelDocumentForm(w, r)
	if err != nil {
		travelFormError(w, r, err)
		return
	}
	resp, err := h.documents.UpdateDocument(r.Context(), p, id, f)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}

// TravelDocumentImage handles GET /travel-documents/{documentId}/image.
func (h *TravelHandler) TravelDocumentImage(w http.ResponseWriter, r *http.Request) {
	p, ok := permsFrom(w, r)
	if !ok {
		return
	}
	id, err := travelPathID(r, "documentId")
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	ct, data, err := h.documents.DocumentImage(r.Context(), p, id)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	travelWriteImage(w, ct, data)
}

// TravellerPassports handles GET /travellers/{email}/passports.
func (h *TravelHandler) TravellerPassports(w http.ResponseWriter, r *http.Request) {
	resp, err := h.documents.Passports(r.Context(), r.PathValue("email"))
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}

// MyPassports handles GET /me/passports.
func (h *TravelHandler) MyPassports(w http.ResponseWriter, r *http.Request) {
	p, ok := permsFrom(w, r)
	if !ok {
		return
	}
	resp, err := h.documents.Passports(r.Context(), p.Email)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}
