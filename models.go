package oushodh

import (
	"encoding/json"
	"reflect"
	"strings"
	"time"
)

// MedicineType is the DGDA register that lists a medicine.
type MedicineType string

const (
	Allopathic   MedicineType = "allopathic"
	Homoeopathic MedicineType = "homoeopathic"
	Herbal       MedicineType = "herbal"
	Ayurvedic    MedicineType = "ayurvedic"
	Unani        MedicineType = "unani"
)

// Medicine is one brand in one strength and dosage form. A value no source gives is nil.
//
// Extra holds the fields the API adds after this version of the SDK, by name.
type Medicine struct {
	ID            string        `json:"id"`
	UUID          string        `json:"uuid"`
	Brand         string        `json:"brand"`
	Category      *string       `json:"category"`
	ProductName   string        `json:"product_name"`
	ProductImage  *string       `json:"product_image"`
	Images        []string      `json:"images"`
	Unit          *string       `json:"unit"`
	Size          *string       `json:"size"`
	Price         *float64      `json:"price"`
	PricePer      *string       `json:"price_per"`
	PriceSource   *string       `json:"price_source"`
	UnitPrices    []UnitPrice   `json:"unit_prices"`
	Prices        Prices        `json:"prices"`
	DescriptionEN *string       `json:"description_en"`
	DescriptionBN *string       `json:"description_bn"`
	DosageForm    *string       `json:"dosage_form"`
	Strength      *string       `json:"strength"`
	GenericName   *string       `json:"generic_name"`
	Manufacturer  *string       `json:"manufacturer"`
	SourceURL     *string       `json:"source_url"`
	LastUpdated   time.Time     `json:"last_updated"`
	DGDAListed    bool          `json:"dgda_listed"`
	MedicineType  *MedicineType `json:"medicine_type"`

	Extra map[string]json.RawMessage `json:"-"`
}

// UnitPrice is the MRP of one unit in one pack size. Kept for older clients: read Prices.Sizes.
type UnitPrice struct {
	Size  *string `json:"size"`
	Price float64 `json:"price"`

	Extra map[string]json.RawMessage `json:"-"`
}

// Prices holds every MRP the sources state for a medicine, in Bangladeshi taka.
type Prices struct {
	Currency string       `json:"currency"`
	Unit     *UnitAmount  `json:"unit"`
	Sizes    []SizeAmount `json:"sizes"`
	Strip    *StripAmount `json:"strip"`
	Packs    []PackAmount `json:"packs"`

	Extra map[string]json.RawMessage `json:"-"`
}

// UnitAmount is the MRP of one unit, such as a tablet or a bottle.
type UnitAmount struct {
	Amount float64 `json:"amount"`
	Source string  `json:"source"`

	Extra map[string]json.RawMessage `json:"-"`
}

// SizeAmount is the unit price in one pack size.
type SizeAmount struct {
	Size   *string `json:"size"`
	Amount float64 `json:"amount"`
	Source string  `json:"source"`

	Extra map[string]json.RawMessage `json:"-"`
}

// StripAmount is the MRP of one strip. Units is nil when the source does not say.
type StripAmount struct {
	Amount float64  `json:"amount"`
	Units  *float64 `json:"units"`
	Source string   `json:"source"`

	Extra map[string]json.RawMessage `json:"-"`
}

// PackAmount is the MRP of one pack or box. Units is nil when the source does not say.
type PackAmount struct {
	Amount float64  `json:"amount"`
	Units  *float64 `json:"units"`
	Label  string   `json:"label"`
	Source string   `json:"source"`

	Extra map[string]json.RawMessage `json:"-"`
}

// MedicineDetail is one medicine with its medical text. Monograph is nil when no source gives any text.
type MedicineDetail struct {
	Medicine
	Monograph *Monograph `json:"monograph"`
}

// Monograph is the medical text of a medicine, as its source wrote it, in English. Some text holds HTML tags.
type Monograph struct {
	Source             string  `json:"source"`
	Indications        *string `json:"indications"`
	Composition        *string `json:"composition"`
	Pharmacology       *string `json:"pharmacology"`
	Dosage             *string `json:"dosage"`
	Interactions       *string `json:"interactions"`
	Contraindications  *string `json:"contraindications"`
	SideEffects        *string `json:"side_effects"`
	PregnancyLactation *string `json:"pregnancy_lactation"`
	Precautions        *string `json:"precautions"`
	Overdose           *string `json:"overdose"`
	Storage            *string `json:"storage"`

	Extra map[string]json.RawMessage `json:"-"`
}

// Page is one page of medicines. Next is nil on the last page.
type Page struct {
	Items []Medicine `json:"items"`
	Next  *string    `json:"next"`
	Total int        `json:"total"`

	Extra map[string]json.RawMessage `json:"-"`
}

// SearchResult is the best matches, best first, and the count of every match.
type SearchResult struct {
	Items []Medicine `json:"items"`
	Total int        `json:"total"`

	Extra map[string]json.RawMessage `json:"-"`
}

// Meta is the account a key belongs to and the data it reads.
type Meta struct {
	Client     string     `json:"client"`
	IDProfile  string     `json:"id_profile"`
	Medicines  int        `json:"medicines"`
	LastImport *time.Time `json:"last_import"`

	Extra map[string]json.RawMessage `json:"-"`
}

func (m *Medicine) UnmarshalJSON(data []byte) error {
	type plain Medicine

	return decode(data, (*plain)(m), &m.Extra)
}

func (d *MedicineDetail) UnmarshalJSON(data []byte) error {
	var rest struct {
		Monograph *Monograph `json:"monograph"`
	}

	if err := json.Unmarshal(data, &d.Medicine); err != nil {
		return err
	}

	if err := json.Unmarshal(data, &rest); err != nil {
		return err
	}

	d.Monograph = rest.Monograph
	delete(d.Extra, "monograph")

	if len(d.Extra) == 0 {
		d.Extra = nil
	}

	return nil
}

func (u *UnitPrice) UnmarshalJSON(data []byte) error {
	type plain UnitPrice

	return decode(data, (*plain)(u), &u.Extra)
}

func (p *Prices) UnmarshalJSON(data []byte) error {
	type plain Prices

	return decode(data, (*plain)(p), &p.Extra)
}

func (a *UnitAmount) UnmarshalJSON(data []byte) error {
	type plain UnitAmount

	return decode(data, (*plain)(a), &a.Extra)
}

func (a *SizeAmount) UnmarshalJSON(data []byte) error {
	type plain SizeAmount

	return decode(data, (*plain)(a), &a.Extra)
}

func (a *StripAmount) UnmarshalJSON(data []byte) error {
	type plain StripAmount

	return decode(data, (*plain)(a), &a.Extra)
}

func (a *PackAmount) UnmarshalJSON(data []byte) error {
	type plain PackAmount

	return decode(data, (*plain)(a), &a.Extra)
}

func (m *Monograph) UnmarshalJSON(data []byte) error {
	type plain Monograph

	return decode(data, (*plain)(m), &m.Extra)
}

func (p *Page) UnmarshalJSON(data []byte) error {
	type plain Page

	return decode(data, (*plain)(p), &p.Extra)
}

func (s *SearchResult) UnmarshalJSON(data []byte) error {
	type plain SearchResult

	return decode(data, (*plain)(s), &s.Extra)
}

func (m *Meta) UnmarshalJSON(data []byte) error {
	type plain Meta

	return decode(data, (*plain)(m), &m.Extra)
}

// decode fills v, then puts every member that has no field in v into extra.
func decode(data []byte, v any, extra *map[string]json.RawMessage) error {
	var all map[string]json.RawMessage

	if err := json.Unmarshal(data, v); err != nil {
		return err
	}

	if err := json.Unmarshal(data, &all); err != nil {
		return err
	}

	t := reflect.TypeOf(v).Elem()

	for i := range t.NumField() {
		name, _, _ := strings.Cut(t.Field(i).Tag.Get("json"), ",")
		delete(all, name)
	}

	if len(all) == 0 {
		all = nil
	}

	*extra = all

	return nil
}
