package database

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

// TestQueryTimeoutDefault verifies the package-level timeout is the
// documented 30s value. Touching this constant should be a deliberate
// decision — a regression here would silently re-enable stuck queries.
func TestQueryTimeoutDefault(t *testing.T) {
	if queryTimeout != 30*time.Second {
		t.Errorf("queryTimeout = %v, want 30s", queryTimeout)
	}
}

// TestQueryTimeoutHonouredByExecuteQuery verifies that a query which
// exceeds queryTimeout is cancelled before its mock delay elapses.
// We shorten queryTimeout to 50ms and have sqlmock delay 500ms so the
// context always wins. The error returned is sqlmock.ErrCancelled; we
// assert it is non-nil and that the elapsed time stayed near the cap.
func TestQueryTimeoutHonouredByExecuteQuery(t *testing.T) {
	orig := queryTimeout
	queryTimeout = 50 * time.Millisecond
	defer func() { queryTimeout = orig }()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	// timeoutTestProvider.PrepareExec is a no-op, so no Exec expectation.
	mock.ExpectQuery("SELECT 1").
		WillDelayFor(500 * time.Millisecond).
		WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(1))

	prov := timeoutTestProvider{}
	start := time.Now()
	_, err = ExecuteQuery(&prov, db, "", "SELECT 1")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("ExecuteQuery returned nil error, want context cancellation")
	}
	// database/sql wraps the context deadline; sqlmock returns its own
	// ErrCancelled sentinel, so check the timing instead of errors.Is.
	if elapsed > 400*time.Millisecond {
		t.Errorf("ExecuteQuery took %v, want < 400ms (timeout must abort early)", elapsed)
	}
	if !isContextDeadlineErr(err) && err.Error() != "canceling query due to user request" {
		t.Errorf("err = %v, want deadline / cancellation", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet mock expectations: %v", err)
	}
}

// TestExecuteStatementHonoursTimeout is the same shape but for the
// non-result-set path (INSERT/UPDATE/DELETE via ExecContext).
func TestExecuteStatementHonoursTimeout(t *testing.T) {
	orig := queryTimeout
	queryTimeout = 50 * time.Millisecond
	defer func() { queryTimeout = orig }()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectExec("INSERT").
		WillDelayFor(500 * time.Millisecond).
		WillReturnResult(sqlmock.NewResult(0, 1))

	prov := timeoutTestProvider{}
	start := time.Now()
	_, err = ExecuteStatement(&prov, db, "", "INSERT INTO t VALUES (1)")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("ExecuteStatement returned nil error, want context cancellation")
	}
	if elapsed > 400*time.Millisecond {
		t.Errorf("ExecuteStatement took %v, want < 400ms", elapsed)
	}
	if !isContextDeadlineErr(err) && err.Error() != "canceling query due to user request" {
		t.Errorf("err = %v, want deadline / cancellation", err)
	}
}

// TestExecuteQuerySuccessWithinDeadline verifies that a fast query is
// not aborted by queryTimeout. This guards against a regression where
// the deadline is applied as an immediate cancel rather than a cap.
func TestExecuteQuerySuccessWithinDeadline(t *testing.T) {
	orig := queryTimeout
	queryTimeout = 5 * time.Second
	defer func() { queryTimeout = orig }()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("SELECT 1").
		WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(1))

	prov := timeoutTestProvider{}
	got, err := ExecuteQuery(&prov, db, "", "SELECT 1")
	if err != nil {
		t.Fatalf("ExecuteQuery: %v", err)
	}
	if len(got.Rows) != 1 {
		t.Errorf("rows = %d, want 1", len(got.Rows))
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet mock expectations: %v", err)
	}
}

// timeoutTestProvider satisfies the Provider interface by embedding
// mysqlProvider (which already implements every method) and overriding
// PrepareExec to a no-op so we never need a real mysql driver.
type timeoutTestProvider struct {
	mysqlProvider
}

func (*timeoutTestProvider) PrepareExec(execer, string) error { return nil }
func (*timeoutTestProvider) DSN(string, int, string, string, string, map[string]string) string {
	return ""
}

// isContextDeadlineErr reports whether err is a context deadline / cancel
// error or has a wrapped one. Used because sqlmock returns a sentinel
// ErrCancelled that does not wrap context.DeadlineExceeded.
func isContextDeadlineErr(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled)
}

// Silence unused-import warnings on the test helpers.
var _ = errors.New
var _ = context.Background