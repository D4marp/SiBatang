package domain

import "regexp"

const (
	RootOrgCode     = "POLDA_JAWA_TENGAH"
	PilotTenantCode = "SIBATANG_BATANG"
)

var (
	polresCodeRe  = regexp.MustCompile(`^POLRES_[A-Z0-9_]{2,40}$`)
	tenantCodeRe  = regexp.MustCompile(`^[A-Z0-9_]{3,60}$`)
	bookingPrefix = regexp.MustCompile(`^[A-Z]{2,5}$`)
	uuidRe        = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
)

func IsUUID(s string) bool { return uuidRe.MatchString(s) }

// OnboardingInput: parent TIDAK diterima dari klien; selalu root Jateng (10_POLDA_FIRST_PILOT.md §4).
type OnboardingInput struct {
	OrgCode, OrgName, TenantCode, BrandName, BookingPrefix string
}

func (o OnboardingInput) Validate() error {
	if !polresCodeRe.MatchString(o.OrgCode) {
		return ErrValidation("org_code must match POLRES_[A-Z0-9_]")
	}
	if o.OrgCode == "POLRES_UJI_B" {
		// Tenant sintetis hanya boleh di staging lewat seed uji, bukan onboarding produksi.
		return ErrBusinessRule("synthetic test tenants are not provisioned through onboarding")
	}
	if !tenantCodeRe.MatchString(o.TenantCode) {
		return ErrValidation("tenant_code invalid")
	}
	if !bookingPrefix.MatchString(o.BookingPrefix) {
		return ErrValidation("booking_prefix must be 2-5 uppercase letters")
	}
	if len(o.OrgName) < 3 || len(o.OrgName) > 120 || len(o.BrandName) < 2 || len(o.BrandName) > 60 {
		return ErrValidation("org_name/brand_name length invalid")
	}
	return nil
}
