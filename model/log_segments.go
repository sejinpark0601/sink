package model

import (
	"github.com/evergreen-ci/sink/db"
	"github.com/evergreen-ci/sink/db/bsonutil"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

const logSegmentsCollection = "simple.log.segments"

type LogSegment struct {
	// common log information
	ID      bson.ObjectId `bson:"_id"`
	LogID   string        `bson:"log_id"`
	URL     string        `bson:"url"`
	Segment int           `bson:"seg"`
	Bucket  string        `bson:"bucket"`
	KeyName string        `bson:"key"`

	// parsed out information
	Metrics LogMetrics `bson:"metrics"`

	Metadata `bson:"metadata"`

	// internal fields used by methods:
	populated bool
}

var (
	logSegmentDocumentIDKey = bsonutil.MustHaveTag(LogSegment{}, "ID")
	logSegmentLogIDKey      = bsonutil.MustHaveTag(LogSegment{}, "LogID")
	logSegmentURLKey        = bsonutil.MustHaveTag(LogSegment{}, "URL")
	logSegmentKeyNameKey    = bsonutil.MustHaveTag(LogSegment{}, "KeyName")
	logSegmentSegmentIDKey  = bsonutil.MustHaveTag(LogSegment{}, "Segment")
	logSegmentMetricsKey    = bsonutil.MustHaveTag(LogSegment{}, "Metrics")
	logSegmentMetadataKey   = bsonutil.MustHaveTag(LogSegment{}, "Metadata")
)

type LogMetrics struct {
	NumberLines       int            `bson:"lines"`
	UniqueLetters     int            `bson:"letters"`
	LetterFrequencies map[string]int `bson:"frequencies"`
}

var (
	logMetricsNumberLinesKey     = bsonutil.MustHaveTag(LogMetrics{}, "NumberLines")
	logMetricsUniqueLettersKey   = bsonutil.MustHaveTag(LogMetrics{}, "UniqueLetters")
	logMetricsLetterFrequencyKey = bsonutil.MustHaveTag(LogMetrics{}, "LetterFrequencies")
)

func (l *LogSegment) Insert() error {
	if l.ID == "" {
		l.ID = bson.NewObjectId()
	}

	return errors.WithStack(db.Insert(logSegmentsCollection, l))
}

func (l *LogSegment) Find(logID string, segment int) error {
	filter := bson.M{
		logSegmentLogIDKey: logID,
	}

	if segment >= 0 {
		filter[logSegmentSegmentIDKey] = segment
	}

	query := db.Query(filter)

	l.populated = false
	err := query.FindOne(logSegmentsCollection, l)
	if err == mgo.ErrNotFound {
		return nil
	}
	l.populated = true

	if err != nil {
		return errors.Wrapf(err, "problem running log query %+v", query)
	}

	return nil
}

func (l *LogSegment) IsNil() bool { return l.populated }

func (l *LogSegment) Remove() error {
	query := db.Query(bson.M{
		logSegmentDocumentIDKey: l.ID,
	})

	return errors.WithStack(query.RemoveOne(logSegmentsCollection))
}

///////////////////////////////////
//
// slice type queries that return a multiple segments

type LogSegments struct {
	logs      []LogSegment
	populated bool
}

func (l *LogSegments) Find(logID string, sorted bool) error {
	query := db.Query(bson.M{
		logSegmentLogIDKey: logID,
	})

	if sorted {
		query.Sort("-" + logSegmentSegmentIDKey)
	}

	err := query.FindAll(logSegmentsCollection, l.logs)
	l.populated = false
	if err == mgo.ErrNotFound {
		return nil
	}
	l.populated = true

	if err != nil {
		return errors.Wrapf(err, "problem running log query %+v", query)
	}
	return nil
}

func (l *LogSegments) IsNil() bool         { return l.populated }
func (l *LogSegments) Slice() []LogSegment { return l.logs }

func (l *LogSegment) Save() error {
	query := l.Metadata.IsolatedUpdateQuery(logSegmentMetadataKey, l.ID)

	return errors.WithStack(query.Update(logSegmentsCollection, l))
}
