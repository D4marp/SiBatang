package domain

import "testing"

func TestAllowPoldaCannotTouchQueueOrCheckin(t *testing.T) {
	ms := []Membership{{RolePoldaMonitor, ScopePoldaJateng, "polda-1"}}
	res := Resource{TenantID: "t1", SiteID: "s1", CounterID: "c1", PoldaOrgID: "polda-1"}
	if !Allow(ms, PermReportAggregateRead, res) {
		t.Fatal("monitor must read aggregate on descendants")
	}
	for _, p := range []Permission{PermQueueTicketCall, PermQueueTicketManage, PermBookingCheckin, PermAnnouncementClaim, PermTenantMemberManage} {
		if Allow(ms, p, res) {
			t.Fatalf("polda monitor must not hold %s", p)
		}
	}
}

func TestAllowRequiresScopeMatch(t *testing.T) {
	op := []Membership{{RoleServiceOperator, ScopeCounter, "c1"}}
	if !Allow(op, PermQueueTicketCall, Resource{TenantID: "t1", SiteID: "s1", CounterID: "c1"}) {
		t.Fatal("assigned counter must be allowed")
	}
	if Allow(op, PermQueueTicketCall, Resource{TenantID: "t1", SiteID: "s1", CounterID: "c2"}) {
		t.Fatal("other counter must be denied")
	}
	if Allow(op, PermQueueTicketCall, Resource{TenantID: "t1"}) {
		t.Fatal("missing counter scope must be denied")
	}
	// Polres A admin tidak menjangkau tenant B.
	adm := []Membership{{RolePolresAdmin, ScopeTenant, "tA"}}
	if Allow(adm, PermScheduleManage, Resource{TenantID: "tB"}) {
		t.Fatal("tenant A admin must be denied on tenant B")
	}
	if !Allow(adm, PermScheduleManage, Resource{TenantID: "tA"}) {
		t.Fatal("tenant A admin allowed on A")
	}
}

func TestAllowDenyByDefault(t *testing.T) {
	if Allow(nil, PermQueueTicketCall, Resource{TenantID: "t"}) {
		t.Fatal("no memberships => deny")
	}
	unknown := []Membership{{RoleServiceOperator, ScopeType("WORLD"), "x"}}
	if Allow(unknown, PermQueueTicketCall, Resource{TenantID: "x", CounterID: "x"}) {
		t.Fatal("unknown scope type must deny")
	}
	plat := []Membership{{RolePlatformAdmin, ScopeJatengPlatform, "p"}}
	if Allow(plat, PermReportAggregateRead, Resource{TenantID: "t", PoldaOrgID: "p"}) {
		t.Fatal("platform admin must not reach tenant resources")
	}
}

func TestDeviceRoles(t *testing.T) {
	if DeviceAllows("KIOSK", PermQueueTicketCall) {
		t.Fatal("kiosk cannot call")
	}
	if !DeviceAllows("ANNOUNCER", PermAnnouncementClaim) || DeviceAllows("ANNOUNCER", PermBookingCheckin) {
		t.Fatal("announcer role wrong")
	}
	if DeviceAllows("OPERATOR", PermQueueTicketCall) {
		t.Fatal("operator device has no staff authority")
	}
}

func TestTicketStateMachine(t *testing.T) {
	cases := []struct {
		from TicketStatus
		act  TicketAction
		to   TicketStatus
		ok   bool
	}{
		{TicketWaiting, ActCall, TicketCalled, true},
		{TicketCalled, ActStart, TicketServing, true},
		{TicketServing, ActComplete, TicketCompleted, true},
		{TicketCalled, ActNoShow, TicketNoShow, true},
		{TicketCalled, ActRequeue, TicketWaiting, true},
		{TicketCalled, ActRecall, TicketCalled, true},
		{TicketWaiting, ActStart, "", false},
		{TicketCompleted, ActCall, "", false},
		{TicketNoShow, ActCall, "", false},
		{TicketServing, ActNoShow, "", false},
	}
	for _, c := range cases {
		got, err := NextTicketStatus(c.from, c.act)
		if (err == nil) != c.ok || got != c.to {
			t.Errorf("%s+%s => %s,%v", c.from, c.act, got, err)
		}
	}
}

func TestAutoAdvanceGuard(t *testing.T) {
	if !AutoAdvanceEligible(CounterReady, true, true, false) {
		t.Fatal("ready staffed counter should auto advance")
	}
	for name, ok := range map[string]bool{
		"paused":    AutoAdvanceEligible(CounterPaused, true, true, false),
		"serving":   AutoAdvanceEligible(CounterServing, true, true, false),
		"closed":    AutoAdvanceEligible(CounterClosed, true, true, false),
		"disabled":  AutoAdvanceEligible(CounterReady, false, true, false),
		"unstaffed": AutoAdvanceEligible(CounterReady, true, false, false),
		"active":    AutoAdvanceEligible(CounterReady, true, true, true),
	} {
		if ok {
			t.Errorf("%s must not auto advance", name)
		}
	}
}

func TestCounterStates(t *testing.T) {
	if _, err := NextCounterState(CounterServing, CAPause); err == nil {
		t.Fatal("cannot pause while serving")
	}
	if s, err := NextCounterState(CounterServing, CAComplete); err != nil || s != CounterReady {
		t.Fatal("complete returns to ready")
	}
}

func TestDisplayNumber(t *testing.T) {
	if FormatDisplayNumber("SKCK", 25) != "SKCK-025" || FormatDisplayNumber("SIM", 1234) != "SIM-1234" {
		t.Fatal("display number format")
	}
}

func TestVoiceNumberOnlyNeverLeaksName(t *testing.T) {
	if ResolveCallName(NameNumberOnly, true, "Budi Santoso") != nil {
		t.Fatal("NUMBER_ONLY must not return name")
	}
	if ResolveCallName(NamePreferred, false, "Budi Santoso") != nil {
		t.Fatal("no consent => no name")
	}
	n := ResolveCallName(NamePreferred, true, "Budi Santoso")
	if n == nil || *n != "Budi" {
		t.Fatalf("preferred name: %v", n)
	}
	f := ResolveCallName(NameFull, true, "Budi  Santoso<script>")
	if f == nil || *f != "Budi Santososcript" {
		t.Fatalf("full name sanitized: %v", f)
	}
	if ResolveCallName(NameFull, true, "12345 !!!") != nil {
		t.Fatal("name without letters => nil")
	}
	long := "Abcdefghijklmnopqrstuvwxyz Abcdefghij"
	if g := ResolveCallName(NameFull, true, long); g == nil || len([]rune(*g)) > maxCallNameRunes {
		t.Fatal("name must be truncated")
	}
}

func TestUtterance(t *testing.T) {
	name := "Budi"
	got := Utterance(BuildSpeech("SKCK-025", "Loket 2", &name))
	want := "Nomor antrean S K C K, nol dua lima, atas nama Budi, silakan menuju loket dua."
	if got != want {
		t.Fatalf("\n got %q\nwant %q", got, want)
	}
	got = Utterance(BuildSpeech("SKCK-025", "Loket 2", nil))
	want = "Nomor antrean S K C K, nol dua lima, silakan menuju loket dua."
	if got != want {
		t.Fatalf("\n got %q\nwant %q", got, want)
	}
}

func TestOnboardingValidation(t *testing.T) {
	ok := OnboardingInput{"POLRES_PEKALONGAN", "Polres Pekalongan", "PEKALONGAN_QUEUE", "Antrean Pekalongan", "PKL"}
	if err := ok.Validate(); err != nil {
		t.Fatal(err)
	}
	bad := []OnboardingInput{
		{"POLDA_JAWA_BARAT", "x y z", "ABC", "Brand", "JBR"},
		{"POLRES_batang", "Polres", "ABC", "Brand", "BTG"},
		{"POLRES_UJI_B", "Polres Uji", "UJI_B", "Brand", "UJB"},
		{"POLRES_X1", "Polres", "abc", "Brand", "BTG"},
		{"POLRES_X1", "Polres", "ABC", "Brand", "b"},
	}
	for i, b := range bad {
		if b.Validate() == nil {
			t.Errorf("case %d should fail", i)
		}
	}
}
