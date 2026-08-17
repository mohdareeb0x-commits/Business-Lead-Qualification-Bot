package database

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Collection names — keep in one place so handlers, repositories, and tests
// never disagree.
const (
	CollectionLeads  = "leads"
	CollectionEvents = "lead_events"
)

// Mongo wraps a connected *mongo.Client and the active *mongo.Database.
type Mongo struct {
	Client   *mongo.Client
	Database *mongo.Database
}

// Connect dials MongoDB, pings it, ensures indexes, and returns a wrapper.
// It fails fast on connection / ping errors.
func Connect(ctx context.Context, uri, dbName string) (*Mongo, error) {
	opts := options.Client().
		ApplyURI(uri).
		SetServerSelectionTimeout(10 * time.Second).
		SetAppName("telegram-lead-bot")

	client, err := mongo.Connect(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("mongo connect: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx, nil); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, fmt.Errorf("mongo ping: %w", err)
	}

	m := &Mongo{
		Client:   client,
		Database: client.Database(dbName),
	}

	if err := m.ensureIndexes(ctx); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, fmt.Errorf("ensure indexes: %w", err)
	}
	return m, nil
}

// Disconnect cleanly closes the Mongo client.
func (m *Mongo) Disconnect(ctx context.Context) error {
	if m == nil || m.Client == nil {
		return nil
	}
	return m.Client.Disconnect(ctx)
}

// ensureIndexes creates the indexes the app relies on for query performance
// and audit history. Safe to call repeatedly — Mongo skips existing indexes.
func (m *Mongo) ensureIndexes(ctx context.Context) error {
	leads := m.Database.Collection(CollectionLeads)
	_, err := leads.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "telegram_id", Value: 1}}},
		{Keys: bson.D{{Key: "status", Value: 1}}},
		{Keys: bson.D{{Key: "score", Value: -1}}},
		{Keys: bson.D{{Key: "created_at", Value: -1}}},
	})
	if err != nil {
		return err
	}

	events := m.Database.Collection(CollectionEvents)
	_, err = events.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "lead_id", Value: 1}}},
		{Keys: bson.D{{Key: "created_at", Value: -1}}},
	})
	return err
}
