package worker

import (
	"context"
	"errors"
	"testing"

	summarycoord "trpc.group/trpc-go/trpc-agent-go/enterprise/pkg/summary"
	"trpc.group/trpc-go/trpc-agent-go/enterprise/pkg/tenant"
	"trpc.group/trpc-go/trpc-agent-go/event"
	"trpc.group/trpc-go/trpc-agent-go/session"
)

type summarySessionService struct {
	session.Service
	value *session.Session
	err   error
}

func (s summarySessionService) GetSession(context.Context, session.Key, ...session.Option) (*session.Session, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.value == nil {
		return nil, nil
	}
	return s.value.Clone(), nil
}

func TestWorkerBuildsExactVersionedSummarySchedule(t *testing.T) {
	value := session.NewSession("physical-app", "owner-1", "session-1")
	value.Events = []event.Event{{ID: "event-1"}, {ID: "event-2"}, {ID: "event-3"}}
	w := &Worker{
		tenant:         testTenantForSummary("tenant-a"),
		summaryEnabled: true,
		sessionService: summarySessionService{value: value},
		appName:        "physical-app",
		agentAppID:     "app-1",
		versionID:      "version-1",
	}
	request := &Request{TenantID: "tenant-a", SessionOwnerID: "owner-1", SessionID: "session-1"}
	schedule := w.buildSummarySchedule(context.Background(), request)
	if schedule == nil {
		t.Fatal("summary schedule is nil")
	}
	if err := schedule.Validate(); err != nil {
		t.Fatalf("schedule validation: %v", err)
	}
	if schedule.Key != (summarycoord.Key{TenantID: "tenant-a", AgentAppID: "app-1", SessionOwnerID: "owner-1", SessionID: "session-1"}) ||
		schedule.AgentVersionID != "version-1" || schedule.TargetEventSequence != 3 {
		t.Fatalf("summary schedule=%#v", schedule)
	}
}

func TestWorkerSummaryScheduleFallsBackToDeferredTarget(t *testing.T) {
	w := &Worker{
		tenant:         testTenantForSummary("tenant-a"),
		summaryEnabled: true,
		sessionService: summarySessionService{err: errors.New("session backend unavailable")},
		appName:        "physical-app",
		agentAppID:     "app-1",
		versionID:      "version-1",
	}
	schedule := w.buildSummarySchedule(context.Background(), &Request{
		TenantID: "tenant-a", SessionOwnerID: "owner-1", SessionID: "session-1",
	})
	if schedule == nil || schedule.TargetEventSequence != 0 {
		t.Fatalf("deferred schedule=%#v", schedule)
	}
	if err := schedule.Validate(); err != nil {
		t.Fatalf("deferred schedule validation: %v", err)
	}
}

func testTenantForSummary(id string) *tenant.Tenant {
	return &tenant.Tenant{ID: id}
}
