package main

import (
	"errors"
	"log"
	"net/http"
	"net/url"

	"github.com/rcrowley/go-tigertonic"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

//curl -H "Content-Type: application/json" -XPUT -d '{"P"}' -f -v http://localhost:3000/beacon/{id}

//Server is the name for the server deal wit it
type Server struct {
	Session *mgo.Session // The main session we'll we be cloning
	DBURI   string       // Where the DB is on the network
	dbName  string       // Name of the MongoDB
}

//Art is the struct that represents the structure of an art piece
type Art struct {
	Beacon     Beacon
	Author     string
	Title      string
	ChatroomID string
}

//Beacon is the struct that structures what the data for a beacon will look like
type Beacon struct {
	ProxID  string
	MajorID string
	MinorID string
}

//ArtRoom is a
type ArtRoom struct {
	Art    Art
	Beacon Beacon
}

const (
	dbName = "artroomServer"
	//DBURI for the mongodb
	DBURI = "127.0.0.1"
)

var (
	s = Server{}
	//CollectionNames is the shiz
	CollectionNames = []string{"beacon", "art"}
	beaconIndex     = mgo.Index{
		Key:        []string{"MinorID"},
		Unique:     true,
		DropDups:   true,
		Background: true,
		Sparse:     true,
		Name:       "beaconIndex",
		//ExpireAf
	}

	artIndex = mgo.Index{
		Key:        []string{"Beacon"},
		Unique:     true,
		DropDups:   true,
		Background: true,
		Sparse:     true,
		Name:       "ArtIndex",
		//ExpireAf
	}
	indices    = []mgo.Index{beaconIndex, artIndex}
	hMux       tigertonic.HostServeMux
	mux, nsMux *tigertonic.TrieServeMux
)

func main() {

	log.Println("Server is warming up...")
	initDB()
	initHandlers()
	defer s.Session.Close()
	server := tigertonic.NewServer("localhost:3000", mux)
	log.Fatal(server.ListenAndServe())
}

func initHandlers() {
	mux = tigertonic.NewTrieServeMux()
	mux.Handle(
		"POST",
		"/beacon",
		tigertonic.Marshaled(handlePOSTBeacon),
	)
	//Could use go-metrics to do hot piece of art
	mux.Handle(
		"GET",
		"/beacon/{minorID}",
		tigertonic.Marshaled(handleBeacon),
	)
}

func handlePOSTBeacon(u *url.URL, h http.Header, artPiece *ArtRoom) (status int, responseHeaders http.Header, _ interface{}, err error) {
	err = Insert("beacon", artPiece.Beacon)
	if err != nil {
		//Return a 500
		return 200, h, nil, errors.New("Couldn't insert corresponding beacon.'")
	}

	err = Insert("art", artPiece.Art)
	if err != nil {
		//Return a 500
		return 500, h, nil, errors.New("Couldn't insert corresponding art piece.'")
	}

	return 200, h, nil, nil
}

func handleBeacon(u *url.URL, h http.Header, _ interface{}) (status int, responseHeaders http.Header, art *Art, err error) {
	minorID := u.Query().Get("minorID")
	log.Println(minorID)
	beacon, err := SearchBeaconByID(minorID, 0, -1)
	if err != nil {
		//Beacon not found return 404
		return 404, responseHeaders, nil, errors.New("Beacon not found in db")
	}
	for _, i := range beacon {
		arts, error := SearchArtByBeacon(i, 0, -1)
		if error != nil {
			return 404, responseHeaders, nil, errors.New("Beacon not assigned to art piece")
		}
		return 200, responseHeaders, &arts[0], nil

	}
	return 404, responseHeaders, nil, errors.New("Beacon not found in db")
}

func initDB() {
	s.DBURI = DBURI
	s.dbName = dbName
	s.getSession()
	s.Session.SetSafe(&mgo.Safe{})
	s.Session.SetMode(mgo.Monotonic, true)
	cNames, errors := EnsureIndex(CollectionNames, indices...)
	for k, err := range errors {
		if err != nil {
			log.Fatalf("Indices not taking for %v;%v\n", cNames[k], err)
		}
	}
}

//EnsureIndex ensures that when we store things, we get the expected results
func EnsureIndex(collectionNames []string, indices ...mgo.Index) (s []string, e []error) {
	for j, k := range indices {
		function := func(c *mgo.Collection) error {
			return c.EnsureIndex(k)
		}
		err := withCollection(collectionNames[j], function)
		if err != nil {
			s = append(s, collectionNames[j])
			e = append(e, err)
		}
	}
	return
}

func (s *Server) getSession() *mgo.Session {
	if s.Session == nil {
		var err error
		dialInfo := &mgo.DialInfo{
			Addrs:    []string{s.DBURI},
			Direct:   true,
			FailFast: false,
		}
		s.Session, err = mgo.DialWithInfo(dialInfo)
		if err != nil {
			log.Fatalf("Can't find MongoDB. Is it started? %v\n", err)
		}
	}
	// Returns a copy of the session so we don't waste le resources? Doesn't reuse socket however
	return s.Session.Copy()
}

func withCollection(collection string, fn func(*mgo.Collection) error) error {
	session := s.getSession()
	defer session.Close()
	c := session.DB(s.dbName).C(collection)
	return fn(c)
}

//Insert datum into a specific collection
func Insert(collectionName string, values ...interface{}) error {
	function := func(c *mgo.Collection) error {
		err := c.Insert(values...)
		if err != nil {
			log.Printf("Can't insert document, %v\n", err)
		}
		return err
	}
	return withCollection(collectionName, function)
}

//SearchBeacon searches for a beacon using a passed in struct
func SearchBeacon(q interface{}, skip int, limit int) (searchResults []Beacon, err error) {
	searchResults = []Beacon{}
	query := func(c *mgo.Collection) error {
		function := c.Find(q).Skip(skip).Limit(limit).All(&searchResults)
		if limit < 0 {
			function = c.Find(q).Skip(skip).All(&searchResults)
		}
		return function
	}
	search := func() error {
		return withCollection("beacon", query)
	}
	err = search()
	return
}

//Estimote id
//

//SearchBeaconByID is a
func SearchBeaconByID(beacon string, skip int, limit int) (searchResults []Beacon, err error) {
	if len(beacon) == 20 {
		return SearchBeacon(bson.M{"MinorID": beacon}, skip, limit)
	}
	return nil, errors.New("Not long enough to be a beacon")
}

//SearchArt is a
func SearchArt(q interface{}, skip int, limit int) (searchResults []Art, err error) {
	searchResults = []Art{}
	query := func(c *mgo.Collection) error {
		function := c.Find(q).Skip(skip).Limit(limit).All(&searchResults)
		if limit < 0 {
			function = c.Find(q).Skip(skip).All(&searchResults)
		}
		return function
	}
	search := func() error {
		return withCollection("art", query)
	}
	err = search()
	return
}

//SearchArtByBeacon is a specific version of searchArt
func SearchArtByBeacon(beacon Beacon, skip int, limit int) (searchResults []Art, err error) {
	return SearchArt(bson.M{"Beacon": beacon}, skip, limit)
}
