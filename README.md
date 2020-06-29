# Unexpected null `fullDocument` for `update` change stream event

On investigation flaky test in our codebase I found strange issue: sometimes I
got change stream `update` event with empty `fullDocument` field for existing
document.

I reproduced this behaviour by looping with same collection:
 - drop collection;
 - shard collection;
 - open change stream;
 - run background thread which add ~2000 records and then update ~100 records;
 - read change stream and check then update event has non-empty `fullDocument`.

Environment details:
 - one shard from single-node replicaset;
 - single-node configuration replicaset;
 - 3 mongos.

On my computer, a spike of errors occurs in the first 10-30 iterations. Then the
probability of errors greatly decreases.

I wrote a simple program with Golang for reproducing this issue (`main.go`). This
program uses `go.mongodb.org/mongo-driver v1.3.4` for MongoDB access but originally
this issue was reproduced with `github.com/globalsign/mgo` fork.

## How to reproduce locally
For reproducing you can simply run `./run.sh` in this repository on Linux host
(you need only docker and internet connection).

It show on every `update` event without `fullDocument` for existing document log
line like:
```
time="2020-06-29T12:09:52Z" level=error msg="Found update without fullDocument: {\"_id\": {\"_data\": \"825EF9DA10000010942B022C0100296E5A1004AA912A6620E34D39AD16C7B19795AEE9461E5F6964002C012C0004\"},\"operationType\": \"update\",\"clusterTime\": {\"$timestamp\":{\"t\":\"1593432592\",\"i\":\"4244\"}},\"ns\": {\"db\": \"test\",\"coll\": \"example1593432590\"},\"documentKey\": {\"_id\": {\"$numberInt\":\"150\"}},\"updateDescription\": {\"updatedFields\": {\"d\": \"2020-06-29 12:09:52.649857545 +0000 UTC m=+2.097620388\",\"k\": {\"$numberInt\":\"1287\"},\"u600\": {\"$numberLong\":\"10151\"}},\"removedFields\": []},\"fullDocument\": null}"
```

Also it show total broken events after every iteration like:
```
time="2020-06-29T12:09:52Z" level=info msg="Test pass [test.example1593432590]: 7"
time="2020-06-29T12:09:52Z" level=info msg="  drop collection..."
time="2020-06-29T12:09:53Z" level=info msg="  begin change stream listen..."
time="2020-06-29T12:09:53Z" level=info msg="  run data modifier (end marker: ObjectID(\"5ef9da119d8a55a51f3392d0\"))"
time="2020-06-29T12:09:53Z" level=info msg="  test passed, total errors: 200"
```
