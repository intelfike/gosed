package main

import (
	"bytes"
	"errors"
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

type App struct {
	Users  Users
	Assign *User
	Files  map[string]*File
}

type File struct {
	Name   string
	File   *checkmodfile.File
	Mem    []byte
	Cursor int
	Scroll int
}
type FileMethods interface {
	Update(string)
	Save()
}

// ファイルのメモリ上データを更新する
func (f *File) Update(b []byte) {
	f.Mem = b
}

// メモリ上データをファイルに書き出す
func (f *File) Save() {

}

type User struct {
	Name          string
	AssignChanged bool
	MemChanged    map[string]bool // ファイル名をキーに、変更があっったか
	UsersChanged  bool
}

func (u *User) Init() {
	u.AssignChanged = true
	u.UsersChanged = true
	for k, _ := range u.MemChanged {
		u.MemChanged[k] = true
	}
}

type Users map[string]*User
type UserMethods interface {
	Add(string)
	Remove(string)
}

// 新規ユーザーを作って追加する
func (u Users) Add(name string) error {
	for k, _ := range u {
		if k == name {
			return errors.New(name + ":既にそのユーザー名は使われています")
		}
	}
	u.ChangedUsers()
	user := &User{
		Name:          name,
		UsersChanged:  true,
		AssignChanged: true,
		MemChanged:    map[string]bool{},
	}
	u[name] = user
	if len(u) == 1 {
		u.Assign(name)
	}
	return nil
}
func (u Users) Assign(name string) error {
	user, ok := u[name]
	if !ok {
		return errors.New(name + "そんなユーザはいません")
	}
	app.Assign = user
	u.ChangedAssign()
	return nil
}
func (u Users) ChangedUsers() {
	for _, v := range u {
		v.UsersChanged = true
	}
}
func (u Users) ChangedAssign() {
	for _, v := range u {
		v.AssignChanged = true
	}
}
func (u Users) ChangedMem(filename string) error {
	for _, v := range u {
		v.MemChanged[filename] = true
	}
	fmt.Println(app.Users)
	return nil
}

var (
	mu     sync.Mutex
	editor = new(User)
	// app.Users = map[string]*User{}
	app = &App{
		Users:  map[string]*User{},
		Assign: &User{},
		Files:  map[string]*File{},
	}
)

func init() {
	flag.Parse()
	// ファイルが１つ以上指定されている
	if len(os.Args) <= 1 {
		fmt.Println("gosed [options] [files...]")
		os.Exit(1)
	}
	// 編集対象のファイルたち
	for _, v := range os.Args[1:] {
		cmf, err := checkmodfile.RegistFile(v)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		app.Files[v] = &File{Name: v, File: cmf}
	}

	// 呼びだされたファイルを提供する
	// index.htmなどの編集ファイル一覧などの初期化は頑張る
	handleFunc("/", http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
		uri := strings.Trim(r.RequestURI, "/")
		if uri == "" {
			uri = "index.html"
		}
		bb, err := Asset("data/" + uri)
		if err == nil {
			if uri == "index.html" {
				// cookieで届いたユーザー情報を記録
				name, err := getCookie(r, "user")
				if err == nil {
					err = app.Users.Add(name)
					if err != nil {
						// 画面再描画時にユーザーを未更新状態にする
						app.Users[name].Init()
					}
				}

				// HTMLを加工してリターン
				html, err := createIndexHTML(bb)
				if err != nil {
					fmt.Println("Create error:", err)
					return
				}
				w.Write(html)
				return
			}
			w.Write(bb)
			return
		}
		ff, ok := app.Files[uri]
		if ok {
			ff.File.WriteTo(w)
			return
		}
		fmt.Fprint(w, uri, " is not found")
		fmt.Println(uri, " is not found")
		// fmt.Println(uri)
	})
	handleFunc("/edit/", http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
		uri := strings.Trim(r.RequestURI, "/")
		uri = strings.TrimPrefix(uri, "edit/")
		_, ok := app.Files[uri]
		if !ok {
			fmt.Fprintln(w, uri, "そのファイルは編集できません")
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "file", Value: uri, Path: "/"})
		b, err := Asset("data/edit.html")
		if err != nil {
			fmt.Println("なぜかdata/edit.htmlが見つからない")
			return
		}
		html, err := createEditHTML(b, uri)
		if err != nil {
			fmt.Println("EditのHTML生成エラー: ", err)
			return
		}
		w.Write([]byte(html))
		// cookieで届いたユーザー情報を記録
		name, err := getCookie(r, "user")
		if err == nil {
			err = app.Users.Add(name)
			if err != nil {
				// 画面再描画時にユーザーを未更新状態にする
				app.Users[name].Init()
			}
		}
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
		mes := "Failed"
		err = app.Users.Add(name)
		if err == nil {
			mes = "Successful"
			fmt.Println("regist user:", name)
		}
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
		app.Users.Assign(name)
		// editorChange = true
		w.Write([]byte("Successful"))
	})
	handleFunc("/user/assign/wait", http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
		name, err := getCookie(r, "user")
		if err != nil {
			fmt.Println("userのクッキーが正しくないです")
			fmt.Fprint(w, "userのクッキーが正しくないです")
			return
		}
		user := app.Users[name]
		// fmt.Println(users, name, user)
		for {
			if !user.AssignChanged {
				time.Sleep(time.Second)
				continue
			}
			go func() {
				time.Sleep(time.Second)
				user.AssignChanged = false
			}()
			w.Write([]byte(app.Assign.Name))
			time.Sleep(time.Second / 2)
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
		cmf, ok := app.Files[file]
		if !ok {
			fmt.Println(file, "そんなものはない")
			return
		}
		cmf.File.Save(cmf.Mem)
		w.Write([]byte("Successful"))
		fmt.Println("save", cmf.Name)
		fmt.Println(string(cmf.Mem))
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
		cmf, ok := app.Files[file]
		if !ok {
			fmt.Println(file, "そんなものはない")
			return
		}
		mu.Lock()
		cmf.Mem = b
		mu.Unlock()
		// メモリ情報を一時ファイルに書き出すのを並列して実行
		name, err := getCookie(r, "user")
		if err != nil {
			fmt.Println(err)
			return
		}
		err = app.Users.ChangedMem(file)
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
		cmf, ok := app.Files[file]
		if !ok {
			fmt.Println(file, "そんなものはない")
			return
		}
		name, err := getCookie(r, "user")
		if err != nil {
			fmt.Println(err)
			return
		}
		user, ok := app.Users[name]
		if !ok {
			fmt.Println(name, "なんてユーザーはいない")
			return
		}
		// comet
		for {
			// fmt.Println("mem", user.MemChanged[cmf.Name])
			if !user.MemChanged[cmf.Name] {
				time.Sleep(time.Second / 2)
				continue
			}
			w.Write(cmf.Mem)
			go func() {
				time.Sleep(time.Second)
				user.MemChanged[cmf.Name] = false
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
		cmf, ok := app.Files[file]
		if !ok {
			fmt.Fprint(w, file, "なんてない")
			fmt.Println("/mem/pull error:", file, "なんてない")
			return
		}
		w.Write(cmf.File.Body)
	})

	handleFunc("/users/wait", http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
		name, err := getCookie(r, "user")
		if err != nil {
			fmt.Println("/users/wait:", name)
			return
		}
		user := app.Users[name]
		for {
			if !user.UsersChanged {
				time.Sleep(time.Second)
				continue
			}
			// ユーザーのリストを表示する
			html := ""
			for k, _ := range app.Users {
				checked := ""
				if k == app.Assign.Name {
					checked = " checked"
				}
				html += `<label>
						<input type="radio" name="r1" class="user" value="` + k + `" onclick="assign_send('` + k + `')"` + checked + `>` + k + `
						</label>`
			}
			go func() {
				time.Sleep(time.Second)
				user.UsersChanged = false
			}()
			w.Write([]byte(html))
			time.Sleep(time.Second / 2)
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

func createOldHTML(w http.ResponseWriter, bb []byte) ([]byte, error) {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(bb))
	if err != nil {
		fmt.Println("/ doc error:", err)
		return nil, err
	}

	// ファイルのリストを表示する
	html := ""
	first := true
	for k, _ := range app.Files {
		selected := ""
		if first {
			// とりあえず表示したいファイル
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
		return nil, err
	}
	return []byte(h), err
}

func createIndexHTML(bb []byte) ([]byte, error) {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(bb))
	if err != nil {
		fmt.Println("/ goquery doc error:", err)
		return nil, err
	}
	// ファイルのリストを表示する
	html := ""
	for k, _ := range app.Files {
		html += `<li>
		<a class="file" href="/edit/` + k + `">` + k + `</a>
		</li>` + "\n"
	}
	doc.Find("#files").SetHtml(html)
	h, err := doc.Html()
	if err != nil {
		fmt.Print(err)
		return nil, err
	}
	return []byte(h), nil
}

func createEditHTML(b []byte, filename string) ([]byte, error) {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	doc.Find("title").SetText(filename)
	html, _ := doc.Html()
	return []byte(html), nil
}

func main() {
	fmt.Printf("Start HTTP Server localhost%s\n", *port)
	fmt.Println(http.ListenAndServe(*port, nil))
}
