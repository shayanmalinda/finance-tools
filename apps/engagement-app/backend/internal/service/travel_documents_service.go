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
	"bytes"
	"context"
	"errors"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/apierror"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/engagemententity"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/hrentity"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/domain"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/filestore"
)

// TravelDocumentsService serves visas, travel documents and their images.
type TravelDocumentsService interface {
	// ListVisas is GET /visas, or GET /me/visas with own.
	ListVisas(ctx context.Context, p domain.Perms, q domain.TravelRecordQuery, own bool) (domain.TravelVisaPage, error)
	GetVisa(ctx context.Context, p domain.Perms, id int) (domain.TravelVisa, error)
	// AddVisa is POST /visas, or POST /me/visas with own.
	AddVisa(ctx context.Context, p domain.Perms, f domain.TravelVisaForm, own bool) (domain.TravelVisa, error)
	UpdateVisa(ctx context.Context, p domain.Perms, id int, f domain.TravelVisaForm) (domain.TravelVisa, error)
	// VisaImage returns the image bytes and their content type.
	VisaImage(ctx context.Context, p domain.Perms, id int) (string, []byte, error)

	ListDocuments(ctx context.Context, p domain.Perms, q domain.TravelRecordQuery, own bool) (domain.TravelDocumentPage, error)
	GetDocument(ctx context.Context, p domain.Perms, id int) (domain.TravelDocument, error)
	AddDocument(ctx context.Context, p domain.Perms, f domain.TravelDocumentForm, own bool) (domain.TravelDocument, error)
	UpdateDocument(ctx context.Context, p domain.Perms, id int, f domain.TravelDocumentForm) (domain.TravelDocument, error)
	DocumentImage(ctx context.Context, p domain.Perms, id int) (string, []byte, error)

	// Passports lists a traveller's Active passports, valid ones first.
	Passports(ctx context.Context, email string) (domain.TravelDocumentPage, error)
}

type travelDocumentsService struct {
	entity engagemententity.TravelAPI
	people *travelPeople
	files  filestore.Store
	now    func() time.Time
	// hiddenVisas are the placeholder visas, left out of the visa lists.
	hiddenVisas []int
}

// NewTravelDocumentsService constructs a TravelDocumentsService.
func NewTravelDocumentsService(entity engagemententity.TravelAPI, hr hrentity.Client, files filestore.Store, now func() time.Time, hiddenVisas ...int) TravelDocumentsService {
	if now == nil {
		now = time.Now
	}
	return &travelDocumentsService{entity: entity, people: newTravelPeople(hr), files: files, now: now, hiddenVisas: hiddenVisas}
}

// travelOwnLimit bounds the own lists, which the screens show unpaged.
const travelOwnLimit = 500

// travelMaxBlob bounds a stored image read back: the legacy data-URL text
// of a 5 MB image is about 6.7 MB.
const travelMaxBlob = 16 << 20

var travelDocTypes = []string{"Passport", "Driver License", "Other ID Document"}

func (s *travelDocumentsService) today() string { return s.now().UTC().Format(time.DateOnly) }

func travelValidityOf(status string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "":
		return "", nil
	case "active":
		return engagemententity.TravelValidityActive, nil
	case "expired":
		return engagemententity.TravelValidityExpired, nil
	}
	return "", travelBadRequest("status must be Active or Expired.")
}

// travelMask shows a document number as its first character, five dots and
// its last three characters (N•••••913).
func travelMask(number *string) string {
	r := []rune(strings.TrimSpace(travelDeref(number)))
	switch {
	case len(r) == 0:
		return ""
	case len(r) <= 4:
		return "•••••" + string(r[len(r)-1:])
	}
	return string(r[:1]) + "•••••" + string(r[len(r)-3:])
}

// travelDisplayStatus is Expired for an Active record past its date.
func travelDisplayStatus(stored, until, today string) string {
	if strings.EqualFold(stored, domain.TravelRecordActive) && until != "" && until < today {
		return domain.TravelRecordExpired
	}
	return stored
}

func (s *travelDocumentsService) visa(v engagemententity.TravelVisaRecord, person travelPerson) domain.TravelVisa {
	out := domain.TravelVisa{
		ID: v.ID, Traveller: domain.Person{Email: travelDeref(v.Email), Name: person.Name},
		Country: v.Country, VisaType: v.VisaType, EntryType: v.EntryType, StartDate: v.StartDate,
		EndDate: v.EndDate, IssuedDate: v.IssuedDate,
		Status:   travelDisplayStatus(travelDeref(v.Status), travelDeref(v.EndDate), s.today()),
		HasImage: strings.TrimSpace(travelDeref(v.ImageID)) != "",
	}
	if p := v.Passport; p != nil {
		out.Passport = &domain.TravelPassport{ID: p.ID, Country: p.Country, MaskedNumber: travelMask(p.DocumentNo),
			IssuedDate: p.IssuedDate, ExpiryDate: p.ExpiryDate}
	}
	return out
}

func (s *travelDocumentsService) document(d engagemententity.TravelDocumentRecord, person travelPerson) domain.TravelDocument {
	status := d.Status
	if d.ExpiryDate != nil {
		status = travelDisplayStatus(d.Status, *d.ExpiryDate, s.today())
	}
	return domain.TravelDocument{
		ID: d.ID, Traveller: domain.Person{Email: travelDeref(d.Email), Name: person.Name},
		DocumentType: d.DocumentType, MaskedNumber: travelMask(d.DocumentNo), Country: d.IssuedCountry,
		IssuedDate: d.IssuedDate, ExpiryDate: d.ExpiryDate, Status: status,
		HasImage: strings.TrimSpace(travelDeref(d.ImageID)) != "",
	}
}

func travelIsOwner(p domain.Perms, owner *string) bool {
	return strings.EqualFold(strings.TrimSpace(travelDeref(owner)), p.Email)
}

// ── Visas ─────────────────────────────────────────────────────────────────

// ListVisas ports visa-search / own-visa-search: stored status Active only
// (legacy status "active"), the Active/Expired filter by end date. The
// Expired filter also lists the visas marked Expired by hand, which the
// default and Active lists leave out.
func (s *travelDocumentsService) ListVisas(ctx context.Context, p domain.Perms, q domain.TravelRecordQuery, own bool) (domain.TravelVisaPage, error) {
	validity, err := travelValidityOf(q.Status)
	if err != nil {
		return domain.TravelVisaPage{}, err
	}
	f := engagemententity.TravelVisaFilter{
		Emails: q.Travellers, Countries: q.Countries, Status: domain.TravelRecordActive, Validity: validity,
		Today: s.today(), Search: q.Search, ExcludeIDs: s.hiddenVisas, Limit: q.Limit, Offset: q.Offset,
		IncludeMarkedExpired: validity == engagemententity.TravelValidityExpired,
	}
	if own {
		f.Emails, f.Limit, f.Offset = []string{p.Email}, travelOwnLimit, 0
	}
	page, err := s.entity.SearchVisas(ctx, f)
	if err != nil {
		return domain.TravelVisaPage{}, err
	}
	emails := make([]string, 0, len(page.Items))
	for _, v := range page.Items {
		emails = append(emails, travelDeref(v.Email))
	}
	people := s.people.many(ctx, emails)
	out := domain.TravelVisaPage{Items: make([]domain.TravelVisa, 0, len(page.Items)), Total: page.Total}
	for _, v := range page.Items {
		out.Items = append(out.Items, s.visa(v, s.people.of(people, travelDeref(v.Email))))
	}
	return out, nil
}

func (s *travelDocumentsService) loadVisa(ctx context.Context, id int) (engagemententity.TravelVisaRecord, error) {
	v, err := s.entity.GetVisa(ctx, id)
	if err != nil {
		return engagemententity.TravelVisaRecord{}, travelNotFound(err, "The visa was not found.")
	}
	return v, nil
}

// GetVisa: View Visa Information (9), or the owner with View Own Visa (20).
func (s *travelDocumentsService) GetVisa(ctx context.Context, p domain.Perms, id int) (domain.TravelVisa, error) {
	v, err := s.loadVisa(ctx, id)
	if err != nil {
		return domain.TravelVisa{}, err
	}
	if !p.Travel.Has(travelPrivViewVisaInfo) && !(p.Travel.Has(travelPrivViewOwnVisa) && travelIsOwner(p, v.Email)) {
		return domain.TravelVisa{}, travelForbidden("You do not have permission to view this visa.")
	}
	return s.visa(v, s.people.get(ctx, travelDeref(v.Email))), nil
}

// VisaImage: Travel 9, the owner with 20, or an agent (18) whose agency has
// a travel request using this visa.
func (s *travelDocumentsService) VisaImage(ctx context.Context, p domain.Perms, id int) (string, []byte, error) {
	v, err := s.loadVisa(ctx, id)
	if err != nil {
		return "", nil, err
	}
	allowed := p.Travel.Has(travelPrivViewVisaInfo) || (p.Travel.Has(travelPrivViewOwnVisa) && travelIsOwner(p, v.Email))
	if !allowed && p.Agency != nil && p.Travel.Has(travelPrivAgentRequests) {
		agency, visaID := p.Agency.ID, id
		page, err := s.entity.SearchTravelRequests(ctx, engagemententity.TravelRequestSearch{
			TravelAgentID: &agency, VisaID: &visaID, Limit: 1,
		})
		if err != nil {
			return "", nil, err
		}
		allowed = page.Total > 0
	}
	if !allowed {
		return "", nil, travelForbidden("You do not have permission to view this visa.")
	}
	return s.image(ctx, filestore.VisasContainer, v.ImageID)
}

// travelVisaInput is a visa form merged over the stored visa (edit).
type travelVisaInput struct {
	country, visaType, entryType, issued, start, end, status string
	passportID                                               *int
}

func travelPick(form string, stored *string) string {
	if v := strings.TrimSpace(form); v != "" {
		return v
	}
	return strings.TrimSpace(travelDeref(stored))
}

func travelDatesValid(dates ...string) error {
	for _, d := range dates {
		if d == "" {
			continue
		}
		if _, err := time.Parse(time.DateOnly, d); err != nil {
			return travelBadRequest("Dates must be in the form YYYY-MM-DD.")
		}
	}
	return nil
}

// validateVisa ports the Add/Edit Visa dialog rules: unless the visa is
// set Inactive, every field, the passport and an image are required; the
// start must not be after the end.
func (s *travelDocumentsService) validateVisa(ctx context.Context, owner string, in travelVisaInput, hasImage bool) error {
	if err := travelDatesValid(in.issued, in.start, in.end); err != nil {
		return err
	}
	if in.country == "" || in.visaType == "" || in.entryType == "" || in.issued == "" || in.start == "" || in.end == "" {
		return travelBadRequest("Please check the form again")
	}
	if in.status != domain.TravelRecordInactive && (in.passportID == nil || !hasImage) {
		return travelBadRequest("Please check the form again")
	}
	if in.start > in.end {
		return travelBadRequest("Please check the dates again")
	}
	if in.passportID != nil {
		d, err := s.entity.GetTravelDocument(ctx, *in.passportID)
		if err != nil {
			var up *apierror.UpstreamError
			if errors.As(err, &up) && up.StatusCode == 404 {
				return travelBadRequest("The selected passport was not found.")
			}
			return err
		}
		if !strings.EqualFold(travelDeref(d.Email), owner) || !strings.EqualFold(travelDeref(d.DocumentType), "Passport") {
			return travelBadRequest("Please select one of the traveller's passports.")
		}
	}
	return nil
}

func travelOwnerEmail(p domain.Perms, form string, own bool) (string, error) {
	if own {
		return p.Email, nil
	}
	e := strings.ToLower(strings.TrimSpace(form))
	if e == "" || !strings.Contains(e, "@") {
		return "", travelBadRequest("Please check the form again")
	}
	return e, nil
}

// AddVisa ports employees/visas and employees/own-visas plus the image
// upload: the image is stored first, then the visa is inserted with it (the
// entity sets the traveller's Active visa of the same country Inactive in
// the same transaction).
func (s *travelDocumentsService) AddVisa(ctx context.Context, p domain.Perms, f domain.TravelVisaForm, own bool) (domain.TravelVisa, error) {
	owner, err := travelOwnerEmail(p, f.TravellerEmail, own)
	if err != nil {
		return domain.TravelVisa{}, err
	}
	in := travelVisaInput{country: strings.TrimSpace(f.Country), visaType: strings.TrimSpace(f.VisaType),
		entryType: strings.TrimSpace(f.EntryType), issued: strings.TrimSpace(f.IssuedDate),
		start: strings.TrimSpace(f.StartDate), end: strings.TrimSpace(f.EndDate),
		status: domain.TravelRecordActive, passportID: f.PassportID}
	if err := s.validateVisa(ctx, owner, in, f.Image.Present); err != nil {
		return domain.TravelVisa{}, err
	}
	name, err := s.store(ctx, filestore.VisasContainer, owner, in.country, f.Image)
	if err != nil {
		return domain.TravelVisa{}, err
	}
	v, err := s.entity.CreateVisa(ctx, engagemententity.TravelVisaCreate{
		Email: owner, Country: in.country, VisaType: in.visaType, EntryType: in.entryType, IssuedDate: in.issued,
		StartDate: in.start, EndDate: in.end, PassportID: in.passportID, ImageID: name, CreatedBy: p.Email,
	})
	if err != nil {
		return domain.TravelVisa{}, err
	}
	return s.visa(v, s.people.get(ctx, owner)), nil
}

// UpdateVisa ports update-visa plus the image upload (optional on edit).
// Empty form fields keep the stored values. Setting Expired while the end
// date is still ahead sets the end date to today (Release 1 dialog rule).
func (s *travelDocumentsService) UpdateVisa(ctx context.Context, p domain.Perms, id int, f domain.TravelVisaForm) (domain.TravelVisa, error) {
	cur, err := s.loadVisa(ctx, id)
	if err != nil {
		return domain.TravelVisa{}, err
	}
	owner := strings.TrimSpace(travelDeref(cur.Email))
	in := travelVisaInput{country: travelPick(f.Country, cur.Country), visaType: travelPick(f.VisaType, cur.VisaType),
		entryType: travelPick(f.EntryType, cur.EntryType), issued: travelPick(f.IssuedDate, cur.IssuedDate),
		start: travelPick(f.StartDate, cur.StartDate), end: travelPick(f.EndDate, cur.EndDate),
		status: travelPick(f.Status, cur.Status), passportID: f.PassportID}
	if in.passportID == nil {
		in.passportID = cur.PassportID
	}
	statuses := []string{domain.TravelRecordActive, domain.TravelRecordInactive, domain.TravelRecordExpired}
	idx := slices.IndexFunc(statuses, func(st string) bool { return strings.EqualFold(st, in.status) })
	if idx < 0 {
		return domain.TravelVisa{}, travelBadRequest("Visa Status must be Active, Inactive or Expired.")
	}
	in.status = statuses[idx]
	if err := travelDatesValid(in.end); err != nil {
		return domain.TravelVisa{}, err
	}
	if today := s.today(); in.status == domain.TravelRecordExpired && in.end > today {
		in.end = today
		if in.start > in.end {
			in.start = in.end
		}
	}
	hasImage := f.Image.Present || strings.TrimSpace(travelDeref(cur.ImageID)) != ""
	if err := s.validateVisa(ctx, owner, in, hasImage); err != nil {
		return domain.TravelVisa{}, err
	}
	write := engagemententity.TravelVisaUpdate{
		Country: in.country, VisaType: in.visaType, EntryType: in.entryType, IssuedDate: in.issued,
		StartDate: in.start, EndDate: in.end, PassportID: f.PassportID, Status: in.status, UpdatedBy: p.Email,
	}
	if f.Image.Present {
		name, err := s.store(ctx, filestore.VisasContainer, owner, in.country, f.Image)
		if err != nil {
			return domain.TravelVisa{}, err
		}
		write.ImageID = &name
	}
	v, err := s.entity.UpdateVisa(ctx, id, write)
	if err != nil {
		return domain.TravelVisa{}, travelNotFound(err, "The visa was not found.")
	}
	return s.visa(v, s.people.get(ctx, owner)), nil
}

// ── Travel documents ──────────────────────────────────────────────────────

// ListDocuments ports travel-document-search / own-travel-document-search.
func (s *travelDocumentsService) ListDocuments(ctx context.Context, p domain.Perms, q domain.TravelRecordQuery, own bool) (domain.TravelDocumentPage, error) {
	validity, err := travelValidityOf(q.Status)
	if err != nil {
		return domain.TravelDocumentPage{}, err
	}
	f := engagemententity.TravelDocumentFilter{
		Emails: q.Travellers, Countries: q.Countries, DocumentTypes: q.DocumentTypes, Status: domain.TravelRecordActive,
		Validity: validity, Today: s.today(), Search: q.Search, Limit: q.Limit, Offset: q.Offset,
	}
	if own {
		f.Emails, f.Limit, f.Offset = []string{p.Email}, travelOwnLimit, 0
	}
	return s.documents(ctx, f)
}

func (s *travelDocumentsService) documents(ctx context.Context, f engagemententity.TravelDocumentFilter) (domain.TravelDocumentPage, error) {
	page, err := s.entity.SearchTravelDocuments(ctx, f)
	if err != nil {
		return domain.TravelDocumentPage{}, err
	}
	emails := make([]string, 0, len(page.Items))
	for _, d := range page.Items {
		emails = append(emails, travelDeref(d.Email))
	}
	people := s.people.many(ctx, emails)
	out := domain.TravelDocumentPage{Items: make([]domain.TravelDocument, 0, len(page.Items)), Total: page.Total}
	for _, d := range page.Items {
		out.Items = append(out.Items, s.document(d, s.people.of(people, travelDeref(d.Email))))
	}
	return out, nil
}

// Passports lists the Active passports of email for the visa form: valid
// ones first, expired ones after (the form disables them).
func (s *travelDocumentsService) Passports(ctx context.Context, email string) (domain.TravelDocumentPage, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" || !strings.Contains(email, "@") {
		return domain.TravelDocumentPage{}, travelBadRequest("Invalid Email, Email must be a valid WSO2 email")
	}
	page, err := s.documents(ctx, engagemententity.TravelDocumentFilter{
		Emails: []string{email}, DocumentTypes: []string{"Passport"}, Status: domain.TravelRecordActive,
		Today: s.today(), Limit: travelOwnLimit,
	})
	if err != nil {
		return domain.TravelDocumentPage{}, err
	}
	slices.SortStableFunc(page.Items, func(a, b domain.TravelDocument) int {
		ae, be := a.Status == domain.TravelRecordExpired, b.Status == domain.TravelRecordExpired
		switch {
		case ae == be:
			return 0
		case be:
			return -1
		}
		return 1
	})
	return page, nil
}

func (s *travelDocumentsService) loadDocument(ctx context.Context, id int) (engagemententity.TravelDocumentRecord, error) {
	d, err := s.entity.GetTravelDocument(ctx, id)
	if err != nil {
		return engagemententity.TravelDocumentRecord{}, travelNotFound(err, "The travel document was not found.")
	}
	return d, nil
}

func travelCanSeeDocument(p domain.Perms, d engagemententity.TravelDocumentRecord) bool {
	return p.Travel.Has(travelPrivViewDocInfo) || (p.Travel.Has(travelPrivViewOwnDoc) && travelIsOwner(p, d.Email))
}

// GetDocument: View Travel Document Information (12), or the owner with 23.
// Holders of Edit Travel Document (13) also get the full number.
func (s *travelDocumentsService) GetDocument(ctx context.Context, p domain.Perms, id int) (domain.TravelDocument, error) {
	d, err := s.loadDocument(ctx, id)
	if err != nil {
		return domain.TravelDocument{}, err
	}
	if !travelCanSeeDocument(p, d) {
		return domain.TravelDocument{}, travelForbidden("You do not have permission to view this travel document.")
	}
	out := s.document(d, s.people.get(ctx, travelDeref(d.Email)))
	if p.Travel.Has(travelPrivEditDoc) {
		out.DocumentNumber = d.DocumentNo
	}
	return out, nil
}

func (s *travelDocumentsService) DocumentImage(ctx context.Context, p domain.Perms, id int) (string, []byte, error) {
	d, err := s.loadDocument(ctx, id)
	if err != nil {
		return "", nil, err
	}
	if !travelCanSeeDocument(p, d) {
		return "", nil, travelForbidden("You do not have permission to view this travel document.")
	}
	return s.image(ctx, filestore.TravelDocumentsContainer, d.ImageID)
}

type travelDocInput struct {
	docType, number, country, issued, expiry, status string
}

// validateDocument ports the Add/Edit Document dialog rules.
func travelValidateDocument(in *travelDocInput, hasImage bool) error {
	idx := slices.IndexFunc(travelDocTypes, func(t string) bool { return strings.EqualFold(t, in.docType) })
	if idx < 0 || in.number == "" || in.country == "" || in.issued == "" {
		return travelBadRequest("Please check the form again")
	}
	in.docType = travelDocTypes[idx]
	if err := travelDatesValid(in.issued, in.expiry); err != nil {
		return err
	}
	if in.status != domain.TravelRecordInactive && ((in.expiry == "" && in.docType != "Other ID Document") || !hasImage) {
		return travelBadRequest("Please check the form again")
	}
	if in.expiry != "" && in.issued > in.expiry {
		return travelBadRequest("Please check the dates again")
	}
	return nil
}

func (s *travelDocumentsService) AddDocument(ctx context.Context, p domain.Perms, f domain.TravelDocumentForm, own bool) (domain.TravelDocument, error) {
	owner, err := travelOwnerEmail(p, f.TravellerEmail, own)
	if err != nil {
		return domain.TravelDocument{}, err
	}
	in := travelDocInput{docType: strings.TrimSpace(f.DocumentType), number: strings.TrimSpace(f.DocumentNumber),
		country: strings.TrimSpace(f.Country), issued: strings.TrimSpace(f.IssuedDate),
		expiry: strings.TrimSpace(f.ExpiryDate), status: domain.TravelRecordActive}
	if err := travelValidateDocument(&in, f.Image.Present); err != nil {
		return domain.TravelDocument{}, err
	}
	name, err := s.store(ctx, filestore.TravelDocumentsContainer, owner, in.country, f.Image)
	if err != nil {
		return domain.TravelDocument{}, err
	}
	d, err := s.entity.CreateTravelDocument(ctx, engagemententity.TravelDocumentCreate{
		Email: owner, DocumentNo: in.number, DocumentType: in.docType, IssuedCountry: in.country,
		IssuedDate: in.issued, ExpiryDate: travelOptional(in.expiry), ImageID: name, CreatedBy: p.Email,
	})
	if err != nil {
		return domain.TravelDocument{}, err
	}
	return s.document(d, s.people.get(ctx, owner)), nil
}

// UpdateDocument ports update-travel-document plus the image upload;
// empty form fields keep the stored values.
func (s *travelDocumentsService) UpdateDocument(ctx context.Context, p domain.Perms, id int, f domain.TravelDocumentForm) (domain.TravelDocument, error) {
	cur, err := s.loadDocument(ctx, id)
	if err != nil {
		return domain.TravelDocument{}, err
	}
	owner := strings.TrimSpace(travelDeref(cur.Email))
	stored := cur.Status
	in := travelDocInput{docType: travelPick(f.DocumentType, cur.DocumentType), number: travelPick(f.DocumentNumber, cur.DocumentNo),
		country: travelPick(f.Country, cur.IssuedCountry), issued: travelPick(f.IssuedDate, cur.IssuedDate),
		expiry: travelPick(f.ExpiryDate, cur.ExpiryDate), status: travelPick(f.Status, &stored)}
	switch {
	case strings.EqualFold(in.status, domain.TravelRecordActive):
		in.status = domain.TravelRecordActive
	case strings.EqualFold(in.status, domain.TravelRecordInactive):
		in.status = domain.TravelRecordInactive
	default:
		return domain.TravelDocument{}, travelBadRequest("Status must be Active or Inactive.")
	}
	hasImage := f.Image.Present || strings.TrimSpace(travelDeref(cur.ImageID)) != ""
	if err := travelValidateDocument(&in, hasImage); err != nil {
		return domain.TravelDocument{}, err
	}
	write := engagemententity.TravelDocumentUpdate{
		DocumentNo: travelOptional(strings.TrimSpace(f.DocumentNumber)), DocumentType: in.docType,
		IssuedCountry: in.country, IssuedDate: in.issued, ExpiryDate: travelOptional(in.expiry),
		Status: in.status, UpdatedBy: p.Email,
	}
	if f.Image.Present {
		name, err := s.store(ctx, filestore.TravelDocumentsContainer, owner, in.country, f.Image)
		if err != nil {
			return domain.TravelDocument{}, err
		}
		write.ImageID = &name
	}
	d, err := s.entity.UpdateTravelDocument(ctx, id, write)
	if err != nil {
		return domain.TravelDocument{}, travelNotFound(err, "The travel document was not found.")
	}
	return s.document(d, s.people.get(ctx, owner)), nil
}

// ── Images ────────────────────────────────────────────────────────────────

// travelImageError maps file store errors.
func travelImageError(err error) error {
	switch {
	case errors.Is(err, filestore.ErrNotConfigured):
		return &apierror.ServiceUnavailableError{Msg: "File storage is not configured."}
	case errors.Is(err, filestore.ErrNotFound):
		return &apierror.NotFoundError{Msg: "The image was not found."}
	}
	return err
}

// store checks the image by its content (JPEG or PNG) and stores it as the
// legacy data-URL text under container/{owner}_{uuid}_{country}.{ext}, so
// the legacy app can still read it. It returns the blob name (image_id).
func (s *travelDocumentsService) store(ctx context.Context, container, owner, country string, img domain.TravelImage) (string, error) {
	if !img.Present {
		return "", travelBadRequest("Please check the form again")
	}
	if len(img.Data) > filestore.MaxImageBytes {
		return "", travelBadRequest("The image must be 5 MB or smaller.")
	}
	ct, ext, err := filestore.CheckImage(img.Data)
	if err != nil {
		return "", travelBadRequest("The image must be a JPEG or PNG file.")
	}
	name := filestore.BlobName(owner, country, ext)
	blob := filestore.EncodeLegacy(ct, img.Data)
	if err := s.files.Put(ctx, container+"/"+name, "", bytes.NewReader(blob), int64(len(blob))); err != nil {
		return "", travelImageError(err)
	}
	return name, nil
}

// image reads a stored image and decodes the legacy data-URL text.
func (s *travelDocumentsService) image(ctx context.Context, container string, imageID *string) (string, []byte, error) {
	name := strings.TrimSpace(travelDeref(imageID))
	if name == "" {
		return "", nil, &apierror.NotFoundError{Msg: "No image has been uploaded."}
	}
	rc, _, err := s.files.Get(ctx, container+"/"+name)
	if err != nil {
		return "", nil, travelImageError(err)
	}
	defer rc.Close()
	raw, err := io.ReadAll(io.LimitReader(rc, travelMaxBlob))
	if err != nil {
		return "", nil, err
	}
	ct, data, err := filestore.DecodeLegacy(raw)
	if err != nil {
		return "", nil, err
	}
	switch ct {
	case "image/jpeg", "image/png", "image/gif", "image/webp":
	default:
		ct = "application/octet-stream"
	}
	return ct, data, nil
}
