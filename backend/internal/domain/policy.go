package domain

// Permission mengikuti konvensi domain.resource.action (01_ARCHITECTURE_TENANCY_RBAC.md §4).
type Permission string

const (
	PermQueueTicketCall       Permission = "queue.ticket.call"
	PermQueueTicketManage     Permission = "queue.ticket.manage" // recall/start/complete/no-show
	PermQueueCounterManage    Permission = "queue.counter.manage"
	PermQueueDisplayRead      Permission = "queue.display.read"
	PermBookingCheckin        Permission = "booking.checkin"
	PermReportAggregateRead   Permission = "report.aggregate.read"
	PermDeviceHealthAggregate Permission = "device.health.aggregate.read"
	PermSiteConfigManage      Permission = "site.config.manage"
	PermScheduleManage        Permission = "schedule.manage"
	PermTenantMemberManage    Permission = "tenant.member.manage"
	PermDeviceEnroll          Permission = "device.enroll"
	PermAnnouncementPolicy    Permission = "announcement.policy.manage"
	PermAnnouncementClaim     Permission = "announcement.claim"
	PermAnnouncementAck       Permission = "announcement.ack"
	PermDeviceHealthWrite     Permission = "device.health.write"
	PermOnboardingManage      Permission = "org.onboarding.manage"
	PermOnboardingApprove     Permission = "org.onboarding.approve"
	PermTenantReadiness       Permission = "tenant.readiness.read"
	PermTenantPublish         Permission = "tenant.publish"
	PermAuditRead             Permission = "audit.read"
)

type Role string

const (
	RolePlatformAdmin    Role = "PLATFORM_ADMIN"
	RolePoldaOrgAdmin    Role = "POLDA_ORG_ADMIN"
	RolePoldaMonitor     Role = "POLDA_MONITOR"
	RolePoldaSupervisor  Role = "POLDA_SUPERVISOR"
	RolePolresAdmin      Role = "POLRES_ADMIN"
	RoleSiteManager      Role = "SITE_MANAGER"
	RoleServiceOperator  Role = "SERVICE_OPERATOR"
	RoleReceptionCheckin Role = "RECEPTION_CHECKIN"
	RoleReportViewer     Role = "REPORT_VIEWER"
	RoleDeviceKiosk      Role = "DEVICE_KIOSK"
	RoleDeviceDisplay    Role = "DEVICE_DISPLAY"
	RoleDeviceAnnouncer  Role = "DEVICE_ANNOUNCER"
)

// RolePermissions adalah sumber otoritas grant. Perhatikan larangan eksplisit:
// POLDA_* tidak pernah memiliki queue.ticket.* / booking.checkin; PLATFORM_ADMIN tidak mendapat akses PII.
var RolePermissions = map[Role][]Permission{
	RolePlatformAdmin:   {PermTenantReadiness},
	RolePoldaOrgAdmin:   {PermOnboardingManage, PermOnboardingApprove, PermTenantPublish, PermTenantReadiness, PermReportAggregateRead, PermDeviceHealthAggregate, PermAuditRead},
	RolePoldaMonitor:    {PermReportAggregateRead, PermDeviceHealthAggregate},
	RolePoldaSupervisor: {PermReportAggregateRead, PermDeviceHealthAggregate},
	RolePolresAdmin: {PermReportAggregateRead, PermSiteConfigManage, PermScheduleManage, PermTenantMemberManage,
		PermDeviceEnroll, PermAnnouncementPolicy, PermTenantReadiness, PermQueueDisplayRead, PermAuditRead},
	RoleSiteManager: {PermReportAggregateRead, PermSiteConfigManage, PermScheduleManage, PermDeviceEnroll,
		PermAnnouncementPolicy, PermQueueDisplayRead, PermBookingCheckin, PermQueueCounterManage},
	RoleServiceOperator:  {PermQueueTicketCall, PermQueueTicketManage, PermQueueCounterManage, PermQueueDisplayRead},
	RoleReceptionCheckin: {PermBookingCheckin},
	RoleReportViewer:     {PermReportAggregateRead},
	RoleDeviceKiosk:      {PermBookingCheckin, PermDeviceHealthWrite},
	RoleDeviceDisplay:    {PermQueueDisplayRead, PermDeviceHealthWrite},
	RoleDeviceAnnouncer:  {PermAnnouncementClaim, PermAnnouncementAck, PermDeviceHealthWrite},
}

// ScopeType divalidasi enum; scope tak dikenal = deny.
type ScopeType string

const (
	ScopeJatengPlatform ScopeType = "JATENG_PLATFORM"
	ScopePoldaJateng    ScopeType = "POLDA_JATENG"
	ScopeTenant         ScopeType = "TENANT"
	ScopeSite           ScopeType = "SITE"
	ScopeCounter        ScopeType = "COUNTER"
	ScopeAudioZone      ScopeType = "AUDIO_ZONE"
)

// Membership = penugasan aktif (sudah difilter revoked/expired oleh pemuat).
type Membership struct {
	Role      Role
	ScopeType ScopeType
	ScopeID   string
}

// Resource menyatakan scope sumber daya yang sudah DIVERIFIKASI dari record database, bukan dari input klien.
// PoldaOrgID hanya diisi bila tenant terbukti descendant root Polda tersebut.
type Resource struct {
	TenantID    string
	SiteID      string
	CounterID   string
	AudioZoneID string
	PoldaOrgID  string
}

func roleGrants(r Role, p Permission) bool {
	for _, g := range RolePermissions[r] {
		if g == p {
			return true
		}
	}
	return false
}

func (m Membership) covers(res Resource) bool {
	if m.ScopeID == "" {
		return false
	}
	switch m.ScopeType {
	case ScopeTenant:
		return res.TenantID != "" && m.ScopeID == res.TenantID
	case ScopeSite:
		return res.SiteID != "" && m.ScopeID == res.SiteID
	case ScopeCounter:
		return res.CounterID != "" && m.ScopeID == res.CounterID
	case ScopeAudioZone:
		return res.AudioZoneID != "" && m.ScopeID == res.AudioZoneID
	case ScopePoldaJateng:
		return res.PoldaOrgID != "" && m.ScopeID == res.PoldaOrgID
	case ScopeJatengPlatform:
		return false // platform admin tidak otomatis menjangkau resource tenant
	}
	return false
}

// Allow = role grants ∩ allowed scope ∩ resource. Deny-by-default.
func Allow(ms []Membership, perm Permission, res Resource) bool {
	for _, m := range ms {
		if roleGrants(m.Role, perm) && m.covers(res) {
			return true
		}
	}
	return false
}

// DevicePermissions memetakan tipe perangkat ke role device.
func DeviceRole(deviceType string) (Role, bool) {
	switch deviceType {
	case "KIOSK":
		return RoleDeviceKiosk, true
	case "DISPLAY":
		return RoleDeviceDisplay, true
	case "ANNOUNCER":
		return RoleDeviceAnnouncer, true
	}
	return "", false // OPERATOR device tidak punya wewenang staf; operasi staf butuh sesi individu
}

func DeviceAllows(deviceType string, perm Permission) bool {
	r, ok := DeviceRole(deviceType)
	return ok && roleGrants(r, perm)
}
