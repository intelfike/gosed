package main

import (
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/intelfike/checkmodfile"
)

var (
	port = flag.String("http", ":8888", "HTTP port number.")
)

var (
	files        = map[string]*checkmodfile.File{}
	editFiles    = map[string]*checkmodfile.File{}
	masterText   []byte
	prevText     []byte
	mu           sync.Mutex
	editor       User
	editorChange bool
	users        = map[string]User{}
)

type User struct {
	Name string
}

func init() {
	flag.Parse()
	// ファイルが１つ以上指定されている
	if len(os.Args) <= 1 {
		fmt.Println("gosed [options] [files...]")
		os.Exit(1)
	}
	var err error
	// 編集対象のファイルたち
	editFiles, err = checkmodfile.RegistFiles(os.Args[1:]...)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	// システム関係のファイルたち
	files, err = checkmodfile.RegistFiles(
		"data/index.html",
		"data/style.css",
		"data/script.js",
		"NotoSansCJKjp-hinted/NotoSansMonoCJKjp-Regular.otf",
	)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	// 呼びだされたファイルを提供する
	handleFunc("/", http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
		uri := strings.Trim(r.RequestURI, "/")
		if uri == "" {
			uri = "data/index.html"
		}
		f, ok := files[uri]
		if ok {
			if uri == "data/index.html" {
				// HTMLを加工してリターン
				bb, err := f.GetBytes()
				if err != nil {
					fmt.Println("/ GetBytes error:", err)
					return
				}
				doc, err := goquery.NewDocumentFromReader(bytes.NewReader(bb))
				if err != nil {
					fmt.Println("/ doc error:", err)
					return
				}

				// cookieで届いたユーザー情報を記録
				user, err := r.Cookie("user")
				if err == nil {
					userName, _ := url.PathUnescape(user.Value)
					users[userName] = User{Name: userName}
				}
				// ユーザーのリストを表示する
				html := ""
				for k, _ := range users {
					html += `<label>
						<input type="radio" name="r1" class="user" value="` + k + `" onclick="assign_send('` + k + `')">` + k + `
						</label>`
				}
				doc.Find("#users").SetHtml(html)

				// ファイルのリストを表示する
				html = ""
				first := true
				for k, _ := range editFiles {
					selected := ""
					if first {
						first = false
						http.SetCookie(w, &http.Cookie{Name: "file", Value: k})
						selected = ` id="selected"`
					}
					html += `<div class="file" onclick="switch_file(this)"` + selected + `>` + k + `</div>` + "\n"
				}
				doc.Find("#files").SetHtml(html)
				h, err := doc.Html()
				if err != nil {
					fmt.Print(err)
					return
				}
				w.Write([]byte(h))
				return
			}
			f.WriteTo(w)
			return
		}
		ff, ok := editFiles[uri]
		if ok {
			ff.WriteTo(w)
			return
		}
		fmt.Fprint(w, uri, " is not found")
		fmt.Println(uri, " is not found")
		// fmt.Println(uri)
	})

	// ユーザー登録依頼
	handleFunc("/user/regist", http.MethodPost, func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		b, err := ioutil.ReadAll(r.Body)
		if err != nil {
			fmt.Println(err)
			return
		}
		name := string(b)
		_, ok := users[name]
		mes := "Failed"
		if !ok {
			mes = "Successful"
			fmt.Println("regist user:", name)
		}
		users[name] = User{Name: name}
		w.Write([]byte(mes))
	})

	// ユーザーを編集者にする
	handleFunc("/user/assign/push", http.MethodPost, func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		b, err := ioutil.ReadAll(r.Body)
		if err != nil {
			fmt.Println(err)
			return
		}
		name := string(b)
		editor = users[name]
		editorChange = true
		w.Write([]byte("Successful"))
	})
	handleFunc("/user/assign/pull", http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(editor.Name))
	})
	handleFunc("/user/assign/wait", http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
		for {
			if !editorChange {
				time.Sleep(time.Second)
				continue
			}
			go func() {
				time.Sleep(time.Second)
				editorChange = false
			}()
			time.Sleep(time.Second / 2)
			w.Write([]byte(editor.Name))
			break
		}
	})

	// 送られてきたデータを保存する
	handleFunc("/save", http.MethodPost, func(w http.ResponseWriter, r *http.Request) {
		file, err := r.Cookie("file")
		if err != nil {
			fmt.Fprint(w, err)
			fmt.Println("save error:", err)
			return
		}
		f, ok := editFiles[file.Value]
		if !ok {
			fmt.Println(file, "そんなものはない")
			return
		}
		f.CommitBody()
		f.Save()
		w.Write([]byte("Successful"))
	})

	// 編集中のデータをメモリ上で共有する
	handleFunc("/mem/push", http.MethodPost, func(w http.ResponseWriter, r *http.Request) {

		// Bodyから編集済みテキストデータを取ってくる
		defer r.Body.Close()
		b, err := ioutil.ReadAll(r.Body)
		if err != nil {
			fmt.Fprint(w, err)
			return
		}
		file, err := getCookie(r, "file")
		if err != nil {
			fmt.Fprint(w, err)
			fmt.Println("/mem/push error:", err)
			return
		}
		f, ok := editFiles[file]
		if !ok {
			fmt.Println(file, "そんなものはない")
			return
		}
		mu.Lock()
		f.Body = b
		mu.Unlock()
		// メモリ情報を一時ファイルに書き出すのを並列して実行
		go func() {
			name, err := getCookie(r, "user")
			if err != nil {
				fmt.Println(err)
				return
			}
			os.Mkdir("tmp/"+name, 0777)
			tmpfile, err := os.Create("tmp/" + name + "/" + file + ".tmp")
			if err != nil {
				fmt.Println(err)
				return
			}
			defer tmpfile.Close()
			tmpfile.Write(b)
		}()
	})

	// メモリの変更があったら返信
	handleFunc("/mem/wait", http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
		file, err := getCookie(r, "file")
		if err != nil {
			fmt.Fprint(w, err)
			fmt.Println("/mem/push error:", err)
			return
		}
		f, ok := editFiles[file]
		if !ok {
			fmt.Println(file, "そんなものはない")
			return
		}
		// comet
		for {
			if f.LatestBody() {
				time.Sleep(time.Second / 2)
				continue
			}
			w.Write(f.Body)
			go func() {
				time.Sleep(time.Second / 2)
				// Bodyの変更をmasterに適用
				f.CommitBody()
			}()
			time.Sleep(time.Second / 5)
			break
		}
	})

	// 待たずにファイル内容を取ってくる
	handleFunc("/mem/pull", http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
		file, err := getCookie(r, "file")
		if err != nil {
			fmt.Fprint(w, err)
			fmt.Println("/mem/pull error:", err)
			return
		}
		f, ok := editFiles[file]
		if !ok {
			fmt.Fprint(w, file, "なんてない")
			fmt.Println("/mem/pull error:", file, "なんてない")
			return
		}
		w.Write(f.Body)
	})
}

func getCookie(r *http.Request, key string) (string, error) {
	cookie, err := r.Cookie(key)
	if err != nil {
		return "", err
	}
	data, err := url.PathUnescape(cookie.Value)
	if err != nil {
		return "", err
	}
	return data, nil
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
	fmt.Printf("Start HTTP Server localhost%s\n", *port)
	fmt.Println(http.ListenAndServe(*port, nil))
}
