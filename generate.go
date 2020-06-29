package main

import (
	"context"
	"fmt"
	"math/rand"
	"sync/atomic"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

type Markers map[int]int64

var g_uid int32
var g_marker int64

func makeDataModification(ctx context.Context, collection *mongo.Collection) error {
	markers := make(Markers)

	if err := shardCollection(ctx, collection, bson.D{{"_id", 1}}); err != nil {
		return err
	}

	if err := DataGenerate(ctx, collection, 2000, func(i int) (int, int) {
		return i, i
	}, markers); err != nil {
		return err
	}

	if err := DataRemove(ctx, collection, 100, func(i int) int {
		return i * 7
	}, markers); err != nil {
		return err
	}

	if err := DataUpdate(ctx, collection, 100, func(i int) (int, int) {
		return (i % 21) * 10, i * 13
	}, markers); err != nil {
		return err
	}
	return nil
}

func DataGenerate(ctx context.Context, collection *mongo.Collection, count int, idGenerator func(i int) (int, int), markers Markers) error {
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

func DataUpdate(ctx context.Context, collection *mongo.Collection, count int, idGenerator func(i int) (int, int), markers Markers) error {
	var bulk []mongo.WriteModel
	for i := 0; i < count; i++ {
		uid := atomic.AddInt32(&g_uid, 1)
		fuid := fmt.Sprintf("u%d", uid)

		id, n := idGenerator(i)
		marker, ok := markers[id]
		if !ok {
			continue
		}

		var data bson.D
		switch rand.Int() % 3 {
		case 0:
			data = bson.D{
				{"$set", bson.D{
					{"k", n},
					{"d", time.Now().String()},
					{fuid, marker},
				}},
			}
		case 1:
			data = bson.D{
				{"$set", bson.D{
					{"k", n},
					{fuid, marker},
				}},
				{"$unset", bson.D{
					{"d", 1},
				}},
			}
		default:
			data = bson.D{
				{"$set", bson.D{
					{fuid, marker},
				}},
				{"$unset", bson.D{
					{"k", 1},
					{"d", 1},
				}},
			}
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
