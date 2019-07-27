package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"strconv"
	"net/http"
	"net"
	"bufio"
	"os"
	"time"
	"github.com/davecgh/go-spew/spew"
	"github.com/gorilla/mux"
	"github.com/joho/godotenv"
)

type Block struct {
	Index int
	Timestamp string
	Heartrate int
	Hash string
	PrevHash string
}

var Blockchain []Block
var bcServer chan []Block
func createHash(fun Block) string {
	data:=string(fun.Index)+fun.Timestamp+string(fun.Heartrate)+fun.PrevHash
	h:=sha256.New()
	h.Write([]byte(data))
	hash:=hex.EncodeToString(h.Sum(nil))
	return hash
}

func createBlock(old Block,BPMdata int) (Block,error) {
	var new Block
	t:=time.Now()
	new.Index=old.Index+1
	new.Timestamp=t.String()
	new.Heartrate=BPMdata
	new.PrevHash=old.Hash
	new.Hash=createHash(new)

	return new, nil
}

func checkBlockValid(new,old Block) bool{
	if new.Index != old.Index+1 {
		return false
	}
	if new.PrevHash != old.Hash {
		return false
	}
	if createHash(new) != new.Hash {
		return false
	}
	return true
}

func longchain(newChain []Block) {
	if len(newChain)>len(Blockchain) {
		Blockchain = newChain
	}
}

func run() error {
	mux:=makeMuxRouter()
	add:=os.Getenv("ADDR")
	s := &http.Server{
		Addr:           ":" + add,
		Handler:        mux,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}
	err:=s.ListenAndServe()
	return err
}

func makeMuxRouter() http.Handler {
	muxrouter:=mux.NewRouter()
	muxrouter.HandleFunc("/",handleGetBlockchain).Methods("GET")
	muxrouter.HandleFunc("/",handleWriteBlock).Methods("POST")
	return muxrouter
}

func handleGetBlockchain(w http.ResponseWriter, r *http.Request) {
	bytes, err := json.MarshalIndent(Blockchain, "", "  ")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	io.WriteString(w, string(bytes))
}

type Message struct{
	BPM int
}

func handleWriteBlock(w http.ResponseWriter, r *http.Request) {
	var m Message

	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&m); err != nil {
		respondWithJSON(w, r, http.StatusBadRequest, r.Body)
		return
	}
	defer r.Body.Close()

	newBlock, err := createBlock(Blockchain[len(Blockchain)-1], m.BPM)
	if err != nil {
		respondWithJSON(w, r, http.StatusInternalServerError, m)
		return
	}
	if checkBlockValid(newBlock,Blockchain[len(Blockchain)-1]) {
		newBlockchain:=append(Blockchain,newBlock)
		longchain(newBlockchain)
		spew.Dump(Blockchain)
	}
	respondWithJSON(w, r, http.StatusCreated, newBlock)
}

func respondWithJSON(w http.ResponseWriter, r *http.Request, code int, payload interface{}) {
	response, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("HTTP 500: Internal Server Error"))
		return
	}
	w.WriteHeader(code)
	w.Write(response)
}

func main(){
	err:=godotenv.Load()
	if err!=nil {
		log.Fatal(err)
	}
	
	bcServer = make(chan []Block)

	t:=time.Now()
	startBlock:=Block{0,t.String(),0,"",""}
	spew.Dump(startBlock)
	Blockchain=append(Blockchain,startBlock)

	server, err := net.Listen("tcp",":"+os.Getenv("ADDR"))
	if err != nil {
		 log.Fatal(err)
	}
	defer server.Close()

	for {
		conn, err := server.Accept()
		if err!=nil {
			log.Fatal(err)
		}
		go handleConn(conn)
	}
}

func handleConn(conn net.Conn) {
	defer conn.Close()

	io.WriteString(conn, "New BPM :")
	scanner := bufio.NewScanner(conn)

	go func() {
		for scanner.Scan() {
			bpm, err := strconv.Atoi(scanner.Text())
			if err != nil {
				log.Printf("%v not a number: %v", scanner.Text(), err)
				continue
			}
			newBlock, err := createBlock(Blockchain[len(Blockchain)-1], bpm)
			if err != nil {
				log.Println(err)
				continue
			}
			if checkBlockValid(newBlock, Blockchain[len(Blockchain)-1]) {
				newBlockchain := append(Blockchain, newBlock)
				longchain(newBlockchain)
			}
			bcServer <- Blockchain
			io.WriteString(conn, "\nEnter a new BPM:")	
		}
	} ()
	go func() {
		for {
			time.Sleep(30 * time.Second)
			output, err := json.Marshal(Blockchain)
			if err != nil {
				log.Fatal(err)
			}
			io.WriteString(conn, string(output))
		}
	}()
	for _ = range bcServer {
		spew.Dump(Blockchain)
	}
}