package main

import (
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
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
	editFiles = map[string]*checkmodfile.File{}
	mu        sync.Mutex
	editor    = new(User)
	users     = map[string]*User{}
	app       = new(App)
)

type App struct {
	Users  map[string]*User
	Assign *User
	Files  map[string]*File
}

type Fileｎ struct {
	File   *checkmodfile.File
	Cursor int
	Scroll int
}

type User struct {
	Name          string
	AssignChanged bool
	MemChanged    map[string]bool // ファイル名をキーに、変更があっったか
	UsersChanged  bool
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

	// 呼びだされたファイルを提供する
	// index.htmなどの編集ファイル一覧などの初期化は頑張る
	handleFunc("/", http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
		uri := strings.Trim(r.RequestURI, "/")
		if uri == "" {
			uri = "data/index.html"
		}
		bb, err := Asset(uri)
		if err == nil {
			if uri == "data/index.html" {
				// HTMLを加工してリターン
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
				userName, err := getCookie(r, "user")
				if err == nil {
					users[userName] = NewUser(userName)
				}

				// ファイルのリストを表示する
				html := ""
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
			w.Write(bb)
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
		users[name] = NewUser(name)
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
		for _, v := range users {
			v.AssignChanged = true
		}
		// editorChange = true
		w.Write([]byte("Successful"))
	})
	handleFunc("/user/assign/wait", http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
		fmt.Println(r.Cookies())
		name, err := getCookie(r, "user")
		if err != nil {
			fmt.Println("userのクッキーが正しくないです")
			fmt.Fprint(w, "userのクッキーが正しくないです")
			return
		}
		user := users[name]
		// fmt.Println(users, name, user)
		for {
			time.Sleep(time.Second)
			if !user.AssignChanged {
				continue
			}
			user.AssignChanged = false
			w.Write([]byte(editor.Name))
			break
		}
	})

	// 送られてきたデータを保存する
	handleFunc("/save", http.MethodPost, func(w http.ResponseWriter, r *http.Request) {
		file, err := getCookie(r, "file")
		if err != nil {
			fmt.Fprint(w, err)
			fmt.Println("save error:", err)
			return
		}
		f, ok := editFiles[file]
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
		name, err := getCookie(r, "user")
		if err != nil {
			fmt.Println(err)
			return
		}
		go func() {
			dir, _ := filepath.Split(file)
			os.MkdirAll("tmp/"+name+"/"+dir, 0777)
			tmpfile, err := os.Create("tmp/" + name + "/" + file + ".backup")
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
			fmt.Println("/mem/wait error:", err)
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

	handleFunc("/users/wait", http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
		name, err := getCookie(r, "user")
		if err != nil {
			fmt.Println("/users/wait:", name)
			return
		}
		user := users[name]
		for {
			time.Sleep(time.Second)
			if !user.UsersChanged {
				continue
			}
			// ユーザーのリストを表示する
			html := ""
			for k, _ := range users {
				checked := ""
				if k == editor.Name {
					checked = " checked"
				}
				html += `<label>
						<input type="radio" name="r1" class="user" value="` + k + `" onclick="assign_send('` + k + `')"` + checked + `>` + k + `
						</label>`
			}
			w.Write([]byte(html))
			break
		}
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

// クッキーをURLデコードして取得する
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
func NewUser(name string) *User {
	user := &User{
		Name:          name,
		UsersChanged:  true,
		AssignChanged: true,
	}
	if len(users) == 0 {
		editor = user
	}
	return user
}

func main() {
	fmt.Printf("Start HTTP Server localhost%s\n", *port)
	fmt.Println(http.ListenAndServe(*port, nil))
}
