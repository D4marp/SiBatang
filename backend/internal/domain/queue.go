package domain

import "fmt"

type TicketStatus string

const (
	TicketWaiting   TicketStatus = "WAITING"
	TicketCalled    TicketStatus = "CALLED"
	TicketServing   TicketStatus = "SERVING"
	TicketCompleted TicketStatus = "COMPLETED"
	TicketNoShow    TicketStatus = "NO_SHOW"
	TicketCancelled TicketStatus = "CANCELLED"
)

type TicketAction string

const (
	ActCall     TicketAction = "CALL"
	ActRecall   TicketAction = "RECALL"
	ActStart    TicketAction = "START"
	ActComplete TicketAction = "COMPLETE"
	ActNoShow   TicketAction = "NO_SHOW"
	ActRequeue  TicketAction = "REQUEUE"
	ActCancel   TicketAction = "CANCEL"
)

var ticketTransitions = map[TicketStatus]map[TicketAction]TicketStatus{
	TicketWaiting: {ActCall: TicketCalled, ActCancel: TicketCancelled},
	TicketCalled:  {ActRecall: TicketCalled, ActStart: TicketServing, ActNoShow: TicketNoShow, ActRequeue: TicketWaiting},
	TicketServing: {ActComplete: TicketCompleted},
}

// NextTicketStatus memvalidasi transisi tiket. Status terminal tidak punya transisi keluar.
func NextTicketStatus(from TicketStatus, act TicketAction) (TicketStatus, error) {
	if to, ok := ticketTransitions[from][act]; ok {
		return to, nil
	}
	return "", ErrStateConflict(fmt.Sprintf("ticket in state %s cannot %s", from, act))
}

type CounterState string

const (
	CounterClosed  CounterState = "CLOSED"
	CounterReady   CounterState = "READY"
	CounterCalling CounterState = "CALLING"
	CounterServing CounterState = "SERVING"
	CounterPaused  CounterState = "PAUSED"
)

// CounterAfter menghitung state loket setelah aksi tiket / aksi operator.
type CounterAction string

const (
	CAMarkReady CounterAction = "READY"
	CAPause     CounterAction = "PAUSE"
	CAClose     CounterAction = "CLOSE"
	CACall      CounterAction = "CALL"
	CAStart     CounterAction = "START"
	CAComplete  CounterAction = "COMPLETE"
	CARelease   CounterAction = "RELEASE" // no-show / requeue
)

var counterTransitions = map[CounterState]map[CounterAction]CounterState{
	CounterClosed:  {CAMarkReady: CounterReady},
	CounterPaused:  {CAMarkReady: CounterReady, CAClose: CounterClosed},
	CounterReady:   {CAMarkReady: CounterReady, CAPause: CounterPaused, CAClose: CounterClosed, CACall: CounterCalling},
	CounterCalling: {CAStart: CounterServing, CARelease: CounterReady, CACall: CounterCalling},
	CounterServing: {CAComplete: CounterReady},
}

func NextCounterState(from CounterState, act CounterAction) (CounterState, error) {
	if to, ok := counterTransitions[from][act]; ok {
		return to, nil
	}
	return "", ErrStateConflict(fmt.Sprintf("counter in state %s cannot %s", from, act))
}

// AutoAdvanceEligible = guard AUTO_ADVANCE_ON_READY (09 §3). Jam booking BUKAN sinyal.
func AutoAdvanceEligible(state CounterState, enabled, staffed, hasActiveTicket bool) bool {
	return enabled && staffed && state == CounterReady && !hasActiveTicket
}

// FormatDisplayNumber: nomor tampil tetap dan tidak pernah dipakai ulang (mis. SKCK-025).
func FormatDisplayNumber(servicePrefix string, serial int) string {
	return fmt.Sprintf("%s-%03d", servicePrefix, serial)
}

// BookingStatus untuk validasi transisi booking (berakhir di CHECKED_IN).
type BookingStatus string

const (
	BookingConfirmed BookingStatus = "CONFIRMED"
	BookingCheckedIn BookingStatus = "CHECKED_IN"
	BookingCancelled BookingStatus = "CANCELLED"
	BookingExpired   BookingStatus = "EXPIRED"
)

func CanCheckIn(s BookingStatus) bool { return s == BookingConfirmed }
func CanCancel(s BookingStatus) bool  { return s == BookingConfirmed }
