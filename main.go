package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/intelfike/checkmodfile"
)

var (
	port     = flag.String("http", ":8888", "HTTP port number.")
	filename = flag.String("file", "", "Edit file")
)

var (
	files      = map[string]*checkmodfile.File{}
	masterText []byte
	prevText   []byte
	mu         sync.Mutex
)

func init() {
	flag.Parse()
	if *filename == "" {
		fmt.Println("-file [edit file name]")
		os.Exit(1)
	}
	var err error

	files, err = checkmodfile.RegistFiles(
		*filename,
		"index.html",
		"NotoSansCJKjp-hinted/NotoSansMonoCJKjp-Regular.otf",
	)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	masterText, err := files[*filename].GetBytes()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	handleFunc("/", http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
		uri := strings.Trim(r.RequestURI, "/")
		if uri == "" {
			uri = "index.html"
		}
		f, ok := files[uri]
		if !ok {
			fmt.Fprint(w, uri, " is not found")
			return
		}
		f.WriteTo(w)
		// fmt.Println(uri)
	})
	handleFunc("/save", http.MethodPost, func(w http.ResponseWriter, r *http.Request) {
		f, err := os.Create(*filename)
		if err != nil {
			fmt.Fprint(w, err)
			return
		}
		defer f.Close()
		f.Write(masterText)
		w.Write([]byte("Successful"))
	})

	// 編集中のデータをメモリ上で共有する
	handleFunc("/mem/push", http.MethodPost, func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		b, err := ioutil.ReadAll(r.Body)
		if err != nil {
			fmt.Fprint(w, err)
			return
		}
		mu.Lock()
		masterText = b
		mu.Unlock()
	})

	// メモリの変更があったら返信
	handleFunc("/mem/wait", http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
		for {
			if string(prevText) == string(masterText) {
				time.Sleep(time.Second / 2)
				continue
			}
			w.Write(masterText)
			go func() {
				time.Sleep(time.Second / 2)
				prevText = masterText
			}()
			time.Sleep(time.Second / 5)
			break
		}
	})

	// // ファイルに保存されたデータを共有する
	// handleFunc("/wait", http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
	// 	for {
	// 		time.Sleep(time.Second / 5)
	// 		latest, err := files[*filename].IsLatest()
	// 		if err != nil {
	// 			fmt.Fprintln(w, err)
	// 			return
	// 		}
	// 		if latest {
	// 			continue
	// 		}
	// 		b, err := files[*filename].GetBytes()
	// 		if err != nil {
	// 			fmt.Println(err)
	// 			fmt.Fprintln(w, err)
	// 			return
	// 		}
	// 		w.Write(b)
	// 		return
	// 	}
	// })

	// 待たずにファイル内容を取ってくる
	handleFunc("/pull", http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
		w.Write(masterText)
	})
}

func handleFunc(path, method string, handler func(http.ResponseWriter, *http.Request)) {
	http.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		// if !strings.HasPrefix(r.RemoteAddr, "127.0.0.1") {
		// 	fmt.Fprint(w, "あなたにアクセス権はありません！")
		// 	return
		// }
		if r.Method != method {
			fmt.Fprint(w, r.Method, " is bad method")
			return
		}
		handler(w, r)
	})
}

func main() {
	fmt.Printf("Start HTTP Server localhost%s", *port)
	fmt.Println(http.ListenAndServe(*port, nil))
}
