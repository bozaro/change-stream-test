package main

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/bsontype"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/x/mongo/driver/connstring"
)

type Markers map[int]int64

var g_uid int32
var g_marker int64

func main() {
	c := fmt.Sprintf("example%d", time.Now().Unix())
	if err := checkOplogLoop(context.Background(), "mongodb://localhost:27017,localhost:27018,localhost:27019/test", c); err != nil {
		panic(err)
	}
}

func checkOplogLoop(ctx context.Context, uri string, c string) error {
	opts := options.Client()
	opts.ApplyURI(uri)

	logrus.Infof("Connecting to: %s (collection: %s)", opts.GetURI(), c)
	cs, err := connstring.Parse(opts.GetURI())
	if err != nil {
		return err
	}
	db := cs.Database
	if db == "" {
		db = "test"
	}

	client, err := mongo.Connect(ctx, opts)
	if err != nil {
		return err
	}
	defer client.Disconnect(context.Background())

	logrus.Infof("Prepare database...")
	if err := enableSharding(ctx, client.Database(db)); err != nil {
		return err
	}

	totalErrors := 0
	for pass := 0; ; pass++ {
		logrus.Infof("Test pass [%s.%s]: %d", db, c, pass)
		logrus.Infof("  drop collection...")
		if err := client.Database(db).Collection(c).Drop(ctx); err != nil {
			return err
		}

		if err := shardCollection(ctx, client.Database(db).Collection(c), bson.D{{"_id", 1}}); err != nil {
			return err
		}

		logrus.Infof("  begin change stream listen...")
		stream, err := client.Database(db).Collection(c).Watch(ctx, mongo.Pipeline{}, options.ChangeStream().
			SetFullDocument(options.UpdateLookup).
			SetBatchSize(2).
			SetMaxAwaitTime(time.Millisecond*100))
		if err != nil {
			return err
		}

		endMarker := primitive.NewObjectID()
		logrus.Infof("  run data modifier (end marker: %s)", endMarker.String())
		go func() {
			defer func() {
				_, err := client.Database(db).Collection(c).InsertOne(ctx, bson.D{{"_id", endMarker}})
				if err != nil {
					logrus.Fatalf("can't insert end marker: %v", err)
				}
			}()

			if err := makeDataModification(ctx, client.Database(db).Collection(c)); err != nil {
				logrus.Errorf("pass error: %v", err)
			}
		}()

		passErrors := 0
		for stream.Next(ctx) {
			event := stream.Current
			opType := event.Lookup("operationType")
			documentKey := event.Lookup("documentKey")
			id := documentKey.Document().Lookup("_id")
			if id.Type == bsontype.ObjectID && id.ObjectID() == endMarker {
				break
			}

			fullDocument := event.Lookup("fullDocument")
			if opType.StringValue() == "update" && fullDocument.Type != bsontype.EmbeddedDocument {
				cur, err := client.Database(db).Collection(c).Find(ctx, documentKey.Document())
				if err == nil {
					if cur.Next(ctx) {
						eventMarker := extractEventMarker(event)
						recordMarker := cur.Current.Lookup(eventMarker.Key())
						if recordMarker.Type != 0 && recordMarker.Int64() == eventMarker.Value().Int64() {
							logrus.Errorf("Found update without fullDocument: %s", event.String())
							passErrors++
						}
					}
					cur.Close(ctx)
				}
			}
		}
		totalErrors += passErrors
		if passErrors == 0 {
			logrus.Infof("  test passed, total errors: %d", totalErrors)
		} else {
			logrus.Infof("  test failed with %d errors, total errors: %d", passErrors, totalErrors)
		}
		stream.Close(context.Background())
	}
	return nil
}

func extractEventMarker(event bson.Raw) bson.RawElement {
	updated := event.Lookup("updateDescription", "updatedFields")
	if updated.Type != bson.TypeEmbeddedDocument {
		return nil
	}
	elements, err := updated.Document().Elements()
	if err != nil {
		return nil
	}
	for _, element := range elements {
		if strings.HasPrefix(element.Key(), "u") {
			return element
		}
	}
	return nil
}

func makeDataModification(ctx context.Context, collection *mongo.Collection) error {
	markers := make(Markers)

	if err := dataGenerate(ctx, collection, 2000, func(i int) (int, int) {
		return i, i
	}, markers); err != nil {
		return err
	}

	if err := dataUpdate(ctx, collection, 100, func(i int) (int, int) {
		return (i % 21) * 10, i * 13
	}, markers); err != nil {
		return err
	}
	return nil
}

func dataGenerate(ctx context.Context, collection *mongo.Collection, count int, idGenerator func(i int) (int, int), markers Markers) error {
	var bulk []mongo.WriteModel
	for i := 0; i < count; i++ {
		id, _ := idGenerator(i)

		marker, ok := markers[id]
		if ok {
			continue
		}
		if !ok {
			marker = atomic.AddInt64(&g_marker, 1)
			markers[id] = marker
		}

		data := bson.D{
			{"_id", id},
			{"u", marker},
		}
		bulk = append(bulk, mongo.NewReplaceOneModel().
			SetFilter(bson.D{{"_id", id}}).
			SetReplacement(data).
			SetUpsert(true))

		if len(bulk) == 1000 {
			if _, err := collection.BulkWrite(ctx, bulk); err != nil {
				return err
			}
			bulk = nil
		}
	}
	if len(bulk) > 0 {
		if _, err := collection.BulkWrite(ctx, bulk); err != nil {
			return err
		}
	}
	return nil
}

func dataUpdate(ctx context.Context, collection *mongo.Collection, count int, idGenerator func(i int) (int, int), markers Markers) error {
	var bulk []mongo.WriteModel
	for i := 0; i < count; i++ {
		uid := atomic.AddInt32(&g_uid, 1)
		fuid := fmt.Sprintf("u%d", uid)

		id, n := idGenerator(i)
		marker, ok := markers[id]
		if !ok {
			continue
		}

		data := bson.D{
			{"$set", bson.D{
				{"k", n},
				{"d", time.Now().String()},
				{fuid, marker},
			}},
		}

		bulk = append(bulk, mongo.NewUpdateOneModel().
			SetFilter(bson.D{{"_id", id}}).
			SetUpdate(data).
			SetUpsert(false))

		if len(bulk) == 1000 {
			if _, err := collection.BulkWrite(ctx, bulk); err != nil {
				return err
			}
			bulk = nil
		}
	}
	if len(bulk) > 0 {
		if _, err := collection.BulkWrite(ctx, bulk); err != nil {
			return err
		}
	}
	return nil
}

func DataRemove(ctx context.Context, collection *mongo.Collection, count int, idGenerator func(i int) int, markers Markers) error {
	var bulk []mongo.WriteModel
	for i := 0; i < count; i++ {
		id := idGenerator(i)
		delete(markers, id)
		bulk = append(bulk, mongo.NewDeleteOneModel().
			SetFilter(bson.D{{"_id", id}}))

		if len(bulk) == 1000 {
			if _, err := collection.BulkWrite(ctx, bulk); err != nil {
				return err
			}
			bulk = nil
		}
	}
	if len(bulk) > 0 {
		if _, err := collection.BulkWrite(ctx, bulk); err != nil {
			return err
		}
	}
	return nil
}

func enableSharding(ctx context.Context, db *mongo.Database) error {
	if res := db.Client().Database("admin").RunCommand(ctx, bson.D{
		{"enableSharding", db.Name()},
	}); res.Err() != nil {
		return res.Err()
	}
	return nil
}

func shardCollection(ctx context.Context, collection *mongo.Collection, keys bson.D) error {
	if res := collection.Database().Client().Database("admin").RunCommand(ctx, bson.D{
		{"shardCollection", collection.Database().Name() + "." + collection.Name()},
		{"key", keys},
	}); res.Err() != nil {
		if e, ok := res.Err().(mongo.CommandError); ok && e.Code == 23 {
			// sharding already enabled for collection
			return nil
		}
		return res.Err()
	}
	return nil
}
