//go:build integration

package postgres_test

import (
	"context"
	"database/sql"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"booking-service/app/models"
	pgstore "booking-service/storage/postgres"
)

func requireDocker(t *testing.T) {
	t.Helper()

	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skipf("Docker недоступен, интеграционные тесты пропущены: %v", err)
	}
}

func setupIntegrationDB(t *testing.T) (*pgxpool.Pool, string, func()) {
	t.Helper()

	requireDocker(t)

	ctx := context.Background()

	container, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("booking"),
		postgres.WithUsername("booking"),
		postgres.WithPassword("booking"),
	)
	require.NoError(t, err)

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	runMigrations(t, dsn)

	pool, err := pgstore.NewPool(ctx, dsn)
	require.NoError(t, err)

	cleanup := func() {
		pool.Close()
		require.NoError(t, container.Terminate(ctx))
	}

	return pool, dsn, cleanup
}

func runMigrations(t *testing.T, dsn string) {
	t.Helper()

	_, filename, _, ok := runtime.Caller(0)
	require.True(t, ok)

	migrationsDir := filepath.Join(filepath.Dir(filename), "../../migrations")

	db, err := goose.OpenDBWithDriver("pgx", dsn)
	require.NoError(t, err)
	defer db.Close()

	require.NoError(t, goose.Up(db, migrationsDir))
}

func indexExists(t *testing.T, db *sql.DB, table, indexName string) bool {
	t.Helper()

	var exists bool
	err := db.QueryRow(`
		SELECT EXISTS (
			SELECT 1
			FROM pg_indexes
			WHERE schemaname = 'public'
			  AND tablename = $1
			  AND indexname = $2
		)`, table, indexName).Scan(&exists)
	require.NoError(t, err)

	return exists
}

func TestPerformanceIndexesExist(t *testing.T) {
	_, dsn, cleanup := setupIntegrationDB(t)
	defer cleanup()

	db := stdlibDB(t, dsn)

	indexes := []struct {
		table string
		name  string
	}{
		// migrations/001
		{"bookings", "idx_bookings_user_id"},
		{"bookings", "idx_bookings_resource_id"},
		{"bookings", "idx_bookings_status"},
		{"bookings", "idx_bookings_id_desc"},
		// migrations/005
		{"outbox_messages", "idx_outbox_messages_retry"},
		// migrations/006
		{"bookings", "idx_bookings_created_at"},
		{"bookings", "idx_bookings_status_created_at"},
		{"bookings", "idx_bookings_status_cancellation_requested_at"},
		{"booking_status_history", "idx_booking_status_history_booking_created"},
		{"outbox_messages", "idx_outbox_messages_retry_event_id"},
	}

	for _, idx := range indexes {
		t.Run(idx.name, func(t *testing.T) {
			require.True(t, indexExists(t, db, idx.table, idx.name),
				"индекс %s на таблице %s не найден", idx.name, idx.table)
		})
	}
}

func TestRepositoryQueriesWorkWithIndexes(t *testing.T) {
	pool, _, cleanup := setupIntegrationDB(t)
	defer cleanup()

	ctx := context.Background()
	repo := pgstore.NewBookingsRepository(pool)

	start := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC)

	booking, err := models.NewBooking(42, 7, start, end)
	require.NoError(t, err)

	history, err := models.NewHistory(
		booking.Status(),
		"",
		0,
		"42",
		models.CauseCreated,
	)
	require.NoError(t, err)

	bookingID, err := repo.CreateWithHistory(ctx, booking, history)
	require.NoError(t, err)
	require.Positive(t, bookingID)

	t.Run("GetByFilter by user_id", func(t *testing.T) {
		userID := int64(42)
		bookings, total, err := repo.GetByFilter(ctx, models.BookingFilter{
			UserID: &userID,
			Page:   1,
			Size:   10,
		})
		require.NoError(t, err)
		require.Equal(t, int64(1), total)
		require.Len(t, bookings, 1)
	})

	t.Run("GetAwaitingConfirmation", func(t *testing.T) {
		bookings, err := repo.GetAwaitingConfirmation(ctx, 10)
		require.NoError(t, err)
		require.NotEmpty(t, bookings)
	})

	t.Run("GetStatistics", func(t *testing.T) {
		from := time.Now().UTC().Add(-time.Hour)
		to := time.Now().UTC().Add(time.Hour)

		stats, err := repo.GetStatistics(ctx, from, to)
		require.NoError(t, err)
		require.GreaterOrEqual(t, stats.TotalCount, int64(1))
	})

	t.Run("GetStuckCancellation", func(t *testing.T) {
		stored, err := repo.GetByID(ctx, bookingID)
		require.NoError(t, err)

		require.NoError(t, stored.BeginCancel(time.Now().UTC()))

		updateHistory, err := models.NewHistory(
			stored.Status(),
			models.BookingStatusAwaitsConfirmation,
			stored.ID(),
			models.InitiatorSystem,
			models.CauseCancellationRequested,
		)
		require.NoError(t, err)

		outbox := &models.BookingStatusChangedEvent{
			EventId:   "11111111-1111-1111-1111-111111111111",
			BookingId: stored.ID(),
			OldStatus: string(models.BookingStatusAwaitsConfirmation),
			NewStatus: string(stored.Status()),
			ChangedAt: time.Now().UTC().Format(time.RFC3339),
			Reason:    models.CauseCancellationRequested,
		}
		require.NoError(t, repo.UpdateWithHistory(ctx, stored, updateHistory, outbox))

		cutoff := time.Now().UTC().Add(time.Hour)
		stuck, err := repo.GetStuckCancellation(ctx, cutoff, 10)
		require.NoError(t, err)
		require.NotNil(t, stuck)
		require.NotEmpty(t, *stuck)
	})

	t.Run("GetOutboxMessages", func(t *testing.T) {
		messages, err := repo.GetOutboxMessages(ctx, 5, 10)
		require.NoError(t, err)
		require.NotEmpty(t, messages)
	})

	t.Run("GetHistoryByBookingID", func(t *testing.T) {
		histories, total, err := repo.GetHistoryByBookingID(ctx, bookingID, 1, 10)
		require.NoError(t, err)
		require.GreaterOrEqual(t, total, int64(2))
		require.NotEmpty(t, histories)
	})
}

func stdlibDB(t *testing.T, dsn string) *sql.DB {
	t.Helper()

	db, err := sql.Open("pgx", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	return db
}
