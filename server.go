package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/nu7hatch/gouuid"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

//curl -H "Content-Type: application/json" -XPUT -d '{"P"}' -f -v http://localhost:3000/beacon/{id}

const (
	dbName = "artroomServer"
	//DBURI for the mongodb
	DBURI = "127.0.0.1"

	waltersAPIPrefix    = "http://api.thewalters.org/v1/objects?apikey=ShxvahaBFNIcfWR7E78xXdssKIlXtUAJk9rDDrrmlvbOlxQKtASCzV4op5aHv2Il&title="
	waltersImagePrefix  = "http://static.thewalters.org/images/"
	waltersImagePostfix = "?width=500"
	moxtraAPI           = "https://api.moxtra.com/oauth/token?client_id="             //client-id
	moxtraCS            = "&client_secret="                                           //CLIENT-SECRET
	moxtraGrantType     = "&grant_type=http://www.moxtra.com/auth_uniqueid&uniqueid=" //UNIQUE-ID
	moxtraTS            = "&timestamp="                                               //TIMESTAMP
	moxtraSig           = "&signature="                                               //SIGNATURE
	moxtraClientID      = "u-DTusZ0ruc"
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
		Key:        []string{"Beacon", "Title"},
		Unique:     true,
		DropDups:   true,
		Background: true,
		Sparse:     true,
		Name:       "ArtIndex",
		//ExpireAf
	}
	indices            = []mgo.Index{beaconIndex, artIndex}
	moxtraTimeStamp    string
	moxtraSignature    []byte
	moxtraClientSecret = []byte("vWLjlkGMZOk")
)

type (
	//Server is the name for the server deal wit it
	Server struct {
		Session     *mgo.Session // The main session we'll we be cloning
		DBURI       string       // Where the DB is on the network
		dbName      string       // Name of the MongoDB
		MoxtraToken MoxtraToken
	}

	//Beacon is the struct that structures what the data for a beacon will look like
	Beacon struct {
		ProxID  string
		MajorID int
		MinorID int
	}
	//FormResponse is what we get back from the form
	FormResponse struct {
		Date        string
		Author      string
		Title       string
		ProxID      string
		MajorID     int
		MinorID     int
		Description string
	}

	//The structure the walter api gives us more or less
	WalterObj struct {
		ObjectID       int
		BinderID       string
		Collection     string
		Title          string
		Author         string
		Medium         string
		Description    string
		Images         string
		CuratorComment string
		Beacon         Beacon
		ImageURL       string
	}
	//Walters objects
	Walters struct {
		Items []WalterObj
	}

	//MoxtraToken balls to the wall
	MoxtraToken struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int    `json:"expires_in"`
		Scope       string
	}

	MoxtraChatRoom struct {
		Name         string
		Conversation bool
	}
	ChatroomResp struct {
		Code string
		Data CRD
	}
	CRD struct {
		ID          string
		Name        string
		Revision    int
		CreatedTime int
		UpdatedTime int
	}
)

func main() {
	log.Println("Server is warming up...")
	initDB()
	initMoxtra()
	defer s.Session.Close()
	http.Handle("/", initHandlers())
	log.Fatalln(http.ListenAndServe(":3000", nil))
}

func initMoxtra() {
	u, err := uuid.NewV4()
	if err != nil {
		log.Fatalf("Unable to Gen UUID for Moxtra.")
	}
	moxtraTimeStamp = strconv.FormatInt(time.Now().Unix()*1000, 10)
	log.Printf("%v", moxtraTimeStamp)
	combString := moxtraClientID + u.String() + moxtraTimeStamp

	mac := hmac.New(sha256.New, moxtraClientSecret)
	mac.Write([]byte(combString))
	moxtraSignature = mac.Sum(nil)
	str := base64.StdEncoding.EncodeToString(moxtraSignature)
	fmt.Println(str)
	str = strings.Replace(str, "\\", "-", -1)
	str = strings.Replace(str, "/", "_", -1)
	str = str[:len(str)-1]
	client := &http.Client{}
	url := moxtraAPI + moxtraClientID + moxtraCS + string(moxtraClientSecret) + moxtraGrantType + u.String() + moxtraTS + moxtraTimeStamp + moxtraSig + str //string(moxtraSignature)
	log.Println(url)
	request, _ := http.NewRequest("POST", url, nil)
	//	request.Header.Add("Accept", "application/json")
	res, err := client.Do(request)
	if err != nil {
		//		http.Error(w, "Req didn't go through", 500)
		//		return
		log.Fatalf("Req didn't go through, %v", err)
	}
	moxie := MoxtraToken{}
	//err = ReadJSON(request, &moxie)
	defer res.Body.Close()
	//	var data json.RawMessage
	log.Printf("%v", res.Body)
	err = json.NewDecoder(res.Body).Decode(&moxie)
	log.Printf("\n\n%v\n", moxie)
	if err != nil {
		//		http.Error(w, "Couldn't decode data...", 500)
		//		return
		log.Fatalf("Can't resolve the datum?\n %v", err)
		//	panic(err)
	}
	log.Println("PASSED")
	//err = json.Unmarshal(data, &moxie)
	if err != nil {
		//http.Error(w, "Couldn't marshall into individual waltOBJ.", 500)
		//return
		log.Fatalf("Can't unmarshall into individ waltOBJ, %v", err)
	}
	s.MoxtraToken = moxie
	log.Printf("%v", moxie)
}

func initHandlers() *mux.Router {
	r := mux.NewRouter()
	r.HandleFunc("/beacon", handlePOSTBeacon).Methods("POST", "OPTIONS")
	r.HandleFunc("/beacon/{minorID:[0-9]+}", handleBeacon).Methods("GET")
	return r
}

func handlePOSTBeacon(w http.ResponseWriter, req *http.Request) {
	log.Println("I got here?")
	switch req.Method {
	case "OPTIONS":
		w.Header().Set("Access-Control-Allow-Headers", "Origin, X-Requested-With, Content-Type, Accept")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		fallthrough
	case "POST":
		//		w.Header().Set("Access-Control-Allow-Origin", "*")
		var formResponse FormResponse
		err := ReadJSON(req, &formResponse)
		log.Printf("We have begun the ritual, %v", formResponse)
		client := &http.Client{}
		request, _ := http.NewRequest("GET", waltersAPIPrefix+strings.Replace(formResponse.Title, " ", "%20", -1), nil)
		request.Header.Add("Accept", "application/json")
		res, err := client.Do(request)
		if err != nil {
			http.Error(w, "Req didn't go through", 500)
			return
			//		log.Fatalf("Req didn't go through, %v", err)
		}
		defer res.Body.Close()
		var data json.RawMessage
		err = json.NewDecoder(res.Body).Decode(&data)
		if err != nil {
			http.Error(w, "Couldn't decode data...", 500)
			return
			//		log.Fatalf("Can't resolve the datum?%v", err)
		}
		wal := Walters{}
		err = json.Unmarshal(data, &wal)
		if err != nil {
			http.Error(w, "Couldn't marshall into individual waltOBJ.", 500)
			return
			//		log.Fatalf("Can't unmarshall into individ waltOBJ, %v", err)
		}
		if len(wal.Items) < 1 {
			http.Error(w, "Couldn't marshall correctly", 500)
			return
		}
		waltersObject := wal.Items[0]
		waltersObject.Beacon = Beacon{formResponse.ProxID, formResponse.MajorID, formResponse.MinorID}
		waltersObject.ImageURL = waltersImagePrefix + string(waltersObject.Images[0]) + waltersImagePostfix
		waltersObject.CuratorComment = formResponse.Description

		name := strconv.Itoa(waltersObject.Beacon.MinorID)
		d, errorsA := json.Marshal(MoxtraChatRoom{Name: name, Conversation: true})
		if errorsA != nil {
			panic(errorsA)
		}
		req2, _ := http.NewRequest("POST", "https://api.moxtra.com/me/binders", bytes.NewBuffer(d))
		req2.Header.Add("Authorization", "Bearer "+s.MoxtraToken.AccessToken)
		req.Header.Set("Content-Type", "application/json")
		res2, err2 := client.Do(req2)
		if err2 != nil {
			http.Error(w, "Req didn't go through", 500)
			return
			//		log.Fatalf("Req didn't go through, %v", err)
		}
		defer res2.Body.Close()
		chr := ChatroomResp{}
		err2 = json.NewDecoder(res2.Body).Decode(&chr)
		if err != nil {
			http.Error(w, "Couldn't decode data...", 500)
			return
			//		log.Fatalf("Can't resolve the datum?%v", err)
		}
		waltersObject.BinderID = chr.Data.ID

		//	err2 = json.Unmarshal(data, &chr)

		err2 = Insert("beacon", waltersObject.Beacon)
		if err2 != nil {
			//Return a 500
			http.Error(w, "Couldn't insert corresponding beacon.", 500)
			return
		}

		err = Insert("art", waltersObject)
		if err != nil {
			//Return a 500
			http.Error(w, "Couldn't insert corresponding art piece.", 500)
			return
		}
		//https//api.moxtra.com/me/binders
		http.Error(w, http.StatusText(http.StatusCreated), http.StatusCreated)
		break
	}
}

func handleBeacon(w http.ResponseWriter, req *http.Request) {
	params := mux.Vars(req)
	minorID, err := strconv.Atoi(params["minorID"])
	if err != nil {
		http.Error(w, "Beacon can't be found with invalid id", 404)
	}
	log.Println(minorID)
	beacon, err := SearchBeaconByID(minorID, 0, -1)
	log.Printf("%v", beacon)
	if err != nil {
		//Beacon not found return 404
		http.Error(w, "Beacon not found in db", 404)
	}
	for _, i := range beacon {
		arts, error := SearchArtByBeacon(i, 0, -1)
		if error != nil {
			http.Error(w, "Beacon assigned to art piece", 404)
		}
		ServeJSON(w, arts)

	}
	//	responseHeaders.Add("Accept","application/json")
	http.Error(w, "Beacon not found", 404)
	//Send a get request to api with keyword param
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
func SearchBeaconByID(minorID int, skip int, limit int) (searchResults []Beacon, err error) {
	//	if len(beacon) == 20 {
	return SearchBeacon(bson.M{"minorid": minorID}, skip, limit)
	//	}
	//	return nil, errors.New("Not long enough to be a beacon")
}

//SearchArt is a
func SearchArt(q interface{}, skip int, limit int) (searchResults []WalterObj, err error) {
	searchResults = []WalterObj{}
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
func SearchArtByBeacon(beacon Beacon, skip int, limit int) (searchResults []WalterObj, err error) {
	return SearchArt(bson.M{"beacon": beacon}, skip, limit)
}

// ServeJSON replies to the request with a JSON
// representation of resource v.
func ServeJSON(w http.ResponseWriter, v interface{}) {
	// avoid json vulnerabilities, always wrap v in an object literal
	//	doc := map[string]interface{}{"d": v}
	if data, err := json.Marshal(v); err != nil {
		log.Printf("Error marshalling json: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	} else {
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Write(data)
	}
}

// ReadJSON decodes JSON data into a provided struct which must be passed in as a pointer.
//If it's not a pointer you are basically putting your data into a bottomless gorge and willing it to
//show up right next to you. Just no.
func ReadJSON(req *http.Request, v interface{}) error {
	defer req.Body.Close()
	decoder := json.NewDecoder(req.Body)
	err := decoder.Decode(v)
	return err
}

//Use this method to debug things
func logRequest(handler func(http.ResponseWriter, *http.Request)) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		var s = time.Now()
		handler(w, r)
		log.Printf("%s %s %6.3fms", r.Method, r.RequestURI, (time.Since(s).Seconds() * 1000))
	}
}
