package auth_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Harshwagh21/NimbusCloud/internal/platform/database/testdb"
	authrepo "github.com/Harshwagh21/NimbusCloud/internal/repository/auth"
	"github.com/Harshwagh21/NimbusCloud/internal/repository/user"
	authsvc "github.com/Harshwagh21/NimbusCloud/internal/service/auth"
)

func TestConcurrentRefreshOneSucceedsAndReuseRevokesTheFamily(t *testing.T) {
	pool := testdb.Open(t)
	testdb.TruncateAll(t, pool)
	svc, err := authsvc.New(user.NewRepository(pool), authrepo.NewRepository(pool), authsvc.Params{
		JWTSecret:       "integration-test-secret-value-32b",
		AccessTokenTTL:  time.Minute,
		RefreshTokenTTL: time.Hour,
		StorageLimit:    10_737_418_240,
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	registered, err := svc.Register(context.Background(), "race@nimbus.test", "Race", "correct-horse")
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	outcomes := refreshTwice(ctx, svc, registered.RefreshToken)
	var winner authsvc.Result
	var successes, reuses int
	for _, outcome := range outcomes {
		switch codeOf(outcome.err) {
		case "":
			successes++
			winner = outcome.result
		case "REFRESH_REUSED":
			reuses++
		default:
			t.Fatalf("refresh: %v", outcome.err)
		}
	}
	if successes != 1 || reuses != 1 {
		t.Fatalf("want one success and one reuse, got %d success %d reuse", successes, reuses)
	}
	if codeOf(mustErr(svc.Refresh(context.Background(), winner.RefreshToken))) != "REFRESH_REUSED" {
		t.Fatal("the racing refresh should have revoked the successor")
	}
}

type refreshOutcome struct {
	result authsvc.Result
	err    error
}

func refreshTwice(ctx context.Context, svc *authsvc.Service, raw string) []refreshOutcome {
	start := make(chan struct{})
	var wg sync.WaitGroup
	outcomes := make([]refreshOutcome, 2)
	for i := range outcomes {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			result, err := svc.Refresh(ctx, raw)
			outcomes[i] = refreshOutcome{result: result, err: err}
		}(i)
	}
	close(start)
	wg.Wait()
	return outcomes
}
