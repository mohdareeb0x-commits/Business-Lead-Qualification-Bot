package lead

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/telegramleadbot/telegram-lead-bot/internal/database"
)

// ErrNotFound is returned by repository methods when a document does not exist.
var ErrNotFound = errors.New("lead not found")

// Repository describes persistence operations for leads and lead events.
// Handlers and services depend on this interface so the storage layer can
// be swapped (e.g. for an integration test fake) without touching callers.
type Repository interface {
	CreateLead(ctx context.Context, l *Lead) error
	GetLeadByID(ctx context.Context, id primitive.ObjectID) (*Lead, error)
	ListLeads(ctx context.Context, q ListLeadsQuery) ([]Lead, int64, error)
	UpdateLeadStatus(ctx context.Context, id primitive.ObjectID, status Status) (*Lead, error)
	CreateEvent(ctx context.Context, e *LeadEvent) error
	ListEvents(ctx context.Context, leadID primitive.ObjectID, page, limit int) ([]LeadEvent, int64, error)
	DashboardStats(ctx context.Context) (*DashboardStats, error)
}

// mongoRepo is the MongoDB-backed implementation of Repository.
type mongoRepo struct {
	leads  *mongo.Collection
	events *mongo.Collection
}

// NewMongoRepository returns a Repository backed by the given Mongo client.
func NewMongoRepository(m *database.Mongo) Repository {
	return &mongoRepo{
		leads:  m.Database.Collection(database.CollectionLeads),
		events: m.Database.Collection(database.CollectionEvents),
	}
}

func (r *mongoRepo) CreateLead(ctx context.Context, l *Lead) error {
	now := time.Now().UTC()
	if l.CreatedAt.IsZero() {
		l.CreatedAt = now
	}
	l.UpdatedAt = now
	if l.Status == "" {
		l.Status = StatusNew
	}
	if l.Source == "" {
		l.Source = SourceTelegram
	}
	if l.ID.IsZero() {
		l.ID = primitive.NewObjectID()
	}
	_, err := r.leads.InsertOne(ctx, l)
	return err
}

func (r *mongoRepo) GetLeadByID(ctx context.Context, id primitive.ObjectID) (*Lead, error) {
	var l Lead
	err := r.leads.FindOne(ctx, bson.M{"_id": id}).Decode(&l)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &l, nil
}

func (r *mongoRepo) ListLeads(ctx context.Context, q ListLeadsQuery) ([]Lead, int64, error) {
	filter := bson.M{}
	if q.Status != "" {
		filter["status"] = q.Status
	}
	if q.Temperature != "" {
		filter["temperature"] = q.Temperature
	}

	total, err := r.leads.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}

	skip := int64((q.Page - 1) * q.Limit)
	findOpts := options.Find().
		SetSkip(skip).
		SetLimit(int64(q.Limit)).
		SetSort(bson.D{{Key: "created_at", Value: -1}})

	cur, err := r.leads.Find(ctx, filter, findOpts)
	if err != nil {
		return nil, 0, err
	}
	defer cur.Close(ctx)

	out := make([]Lead, 0, q.Limit)
	if err := cur.All(ctx, &out); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

func (r *mongoRepo) UpdateLeadStatus(ctx context.Context, id primitive.ObjectID, status Status) (*Lead, error) {
	now := time.Now().UTC()
	after := options.After
	opts := options.FindOneAndUpdateOptions{
		ReturnDocument: &after,
	}
	var updated Lead
	err := r.leads.FindOneAndUpdate(ctx,
		bson.M{"_id": id},
		bson.M{"$set": bson.M{"status": status, "updated_at": now}},
		&opts,
	).Decode(&updated)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &updated, nil
}

func (r *mongoRepo) CreateEvent(ctx context.Context, e *LeadEvent) error {
	if e.ID.IsZero() {
		e.ID = primitive.NewObjectID()
	}
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now().UTC()
	}
	_, err := r.events.InsertOne(ctx, e)
	return err
}

func (r *mongoRepo) ListEvents(ctx context.Context, leadID primitive.ObjectID, page, limit int) ([]LeadEvent, int64, error) {
	filter := bson.M{"lead_id": leadID}
	total, err := r.events.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	skip := int64((page - 1) * limit)
	findOpts := options.Find().
		SetSkip(skip).
		SetLimit(int64(limit)).
		SetSort(bson.D{{Key: "created_at", Value: 1}})

	cur, err := r.events.Find(ctx, filter, findOpts)
	if err != nil {
		return nil, 0, err
	}
	defer cur.Close(ctx)
	out := make([]LeadEvent, 0, limit)
	if err := cur.All(ctx, &out); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

func (r *mongoRepo) DashboardStats(ctx context.Context) (*DashboardStats, error) {
	stats := &DashboardStats{
		ByStatus:      map[string]int64{},
		ByTemperature: map[string]int64{},
	}
	// Total + by-status in a single aggregation.
	pipeline := mongo.Pipeline{
		{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: "$status"},
			{Key: "count", Value: bson.D{{Key: "$sum", Value: 1}}},
		}}},
	}
	cur, err := r.leads.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	type bucket struct {
		ID    string `bson:"_id"`
		Count int64  `bson:"count"`
	}
	var buckets []bucket
	if err := cur.All(ctx, &buckets); err != nil {
		return nil, err
	}
	for _, b := range buckets {
		stats.ByStatus[b.ID] = b.Count
		stats.TotalLeads += b.Count
	}

	// By temperature.
	tempCur, err := r.leads.Aggregate(ctx, mongo.Pipeline{
		{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: "$temperature"},
			{Key: "count", Value: bson.D{{Key: "$sum", Value: 1}}},
		}}},
	})
	if err != nil {
		return nil, err
	}
	var tempBuckets []bucket
	if err := tempCur.All(ctx, &tempBuckets); err != nil {
		return nil, err
	}
	for _, b := range tempBuckets {
		stats.ByTemperature[b.ID] = b.Count
	}

	// Leads created today (UTC).
	startOfDay := time.Now().UTC().Truncate(24 * time.Hour)
	today, err := r.leads.CountDocuments(ctx, bson.M{
		"created_at": bson.M{"$gte": startOfDay},
	})
	if err != nil {
		return nil, err
	}
	stats.LeadsCreatedToday = today

	return stats, nil
}
