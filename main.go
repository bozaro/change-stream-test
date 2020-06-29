package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/bsontype"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/x/mongo/driver/connstring"
)

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
				_, err := client.Database(db).Collection(c).InsertOne(ctx, bson.D{{"_id", endMarker}, {"n", 1}})
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
						recordMarker := cur.Current.Lookup("u")
						eventMarker := extractEventMarker(event)
						if recordMarker.Int64() == eventMarker.Int64() {
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

func extractEventMarker(event bson.Raw) bson.RawValue {
	updated := event.Lookup("updateDescription", "updatedFields")
	if updated.Type != bson.TypeEmbeddedDocument {
		return bson.RawValue{}
	}
	elements, err := updated.Document().Elements()
	if err != nil {
		return bson.RawValue{}
	}
	for _, element := range elements {
		if strings.HasPrefix(element.Key(), "u") {
			return element.Value()
		}
	}
	return bson.RawValue{}
}
