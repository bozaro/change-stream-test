module github.com/bozaro/change-stream-test

go 1.14

require (
	github.com/sirupsen/logrus v1.4.2
	go.mongodb.org/mongo-driver v1.3.4
)

replace go.mongodb.org/mongo-driver v1.3.4 => ../mongo-go-driver
