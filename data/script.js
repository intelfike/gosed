var editor = ""
var user = ""
// 初期化
const init = ()=>{
	editable(false)
	users_wait()
	assign_wait()
	mem_pull()
	mem_wait()
	edit.onclick()
	edit.focus()
	updateLineNum()
}

function must_input_user(){
	user = localStorage.getItem('user')
	if(user != null){
		document.cookie = "user="+encodeURIComponent(user)
		init()
		return
	}
	user = window.prompt("ユーザー名を入力してください")
	if(user == null){
		setTimeout(must_input_user, 1000)
		return
	}
	console.log(user)
	user = user.trim(' ')
	user = user.trim(' ')
	if(user == ''){
		must_input_user()
		return
	}
	var xmlhttp = new XMLHttpRequest()
	xmlhttp.onload = function(){
		var res=xmlhttp.responseText // 受信した文字列
		console.log(res)
		if(res == "Successful"){
			localStorage.setItem('user', user)
			console.log("user=" + encodeURIComponent(user))
			document.cookie = "user="+encodeURIComponent(user)
			init()
			return
		}
		must_input_user()
		return
	}
	xmlhttp.open("POST", "user/regist", true)
	xmlhttp.send(user)
}
document.body.onload = must_input_user

var assign_button = document.getElementById('assign')
assign_button.onclick = ()=>{
	// 自分自身に権限を割り当てる必要がある
	assign_send(user)
}

// ファイルの表示を切り替える
function switch_file(obj){
	document.cookie = "file="+encodeURIComponent(obj.innerText)
	files = document.getElementsByClassName('file')
	for(let n = 0; n < files.length; n++){
		files[n].id = ""
	}
	obj.id = "selected"
	mem_pull()
}

var edit = document.getElementById("edit")
async function save(){
	await mem_send()
	var xmlhttp = new XMLHttpRequest()
	xmlhttp.onload = function(){
		var res=xmlhttp.responseText // 受信した文字列
	}
	xmlhttp.open("POST", "save", true)
	// xmlhttp.setRequestHeader('Content-Type','application/x-www-form-urlencoded')
	xmlhttp.send("")
}
// テキストを更新するときはこれを使う
function updateEdit(text){
	var cursor = edit.selectionStart
	var scroll = edit.scrollTop
	edit.value = text
	edit.selectionStart = cursor
	edit.selectionEnd = cursor
	updateLineNum()
}

edit.onkeydown = function(e){
	if(editor != user){
		return
	}
	switch(e.code){	
	case "KeyS":
		if (e.ctrlKey) {
			console.log("ctrl+s")
			save()
			return false
		}
		break
	case "Tab":
		if (e.ctrlKey) {
			return true
		}
		var text = edit.value
		var cursor = edit.selectionStart
		var scroll = edit.scrollTop
		var s = text.substring(0, cursor)
		var e = text.substring(cursor)
		edit.value = s + "\t" + e
		edit.selectionStart = cursor + 1
		edit.selectionEnd = cursor + 1
		edit.scrollTop = scroll
		return false
		break
	case "Enter":
		updateLineNum()
		break
	case "Backspace":
		updateLineNum()
		break
	}
	console.log(e.code)
}

var timeouter
edit.onkeyup = function(e){
	if(editor != user){
		return
	}
	clearTimeout(timeouter)
	timeouter = setTimeout(mem_send, 500)
	switch(e.code){
	case "Enter":
		updateLineNum()
		break
	case "Backspace":
		updateLineNum()
		break
	}
}
var linenum = document.getElementById("linenum")
function updateLineNum(){
	len = (edit.value.match(/\n/g)||[]).length+1
	linenum.innerHTML = ''
	maxlen = (len + '').length
	for(let n = 1; n <= len; n++){
		// スペースを挟み込む
		var num = '' + n
		var numlen = num.length
		for(let nn = 0; nn < maxlen - numlen; nn++){
			num = '&nbsp;'+num
		}
		linenum.innerHTML += '<div class="num">'+num+'</div>'
	}
}

// 業表示の高さを合わせる
edit.onclick = function(e){
	linenum.style.height = edit.offsetHeight - 4 + "px"
}
edit.onmousemove = edit.onclick

// 行表示のスクロール位置を同期させる
edit.onscroll = function(e){
	linenum.scrollTop = edit.scrollTop
}

function assign_send(user_name){
	var xmlhttp = new XMLHttpRequest()
	xmlhttp.onload = function(){
		var res=xmlhttp.responseText // 受信した文字列
	}
	xmlhttp.open("POST", "user/assign/push", true)
	xmlhttp.send(user_name)
}
function assign_wait(){
	var xmlhttp = new XMLHttpRequest()
	xmlhttp.onload = function(){
		var res = xmlhttp.responseText // 受信した文字列
		console.log(res, user)
		editor = res
		editable(editor == user)
		assign_wait()
	}
	xmlhttp.open("GET", "user/assign/wait", true)
	console.log(document.cookie)
	xmlhttp.send("")

}

// 送ってメモリ上に保存・共有する
function mem_send(){
	return new Promise(ok=>{
		console.log('send')
		// データを同期する
		var xmlhttp = new XMLHttpRequest()
		xmlhttp.onload = function(){
			var res=xmlhttp.responseText // 受信した文字列
		}
		xmlhttp.open("POST", "mem/push", true)
		xmlhttp.send(edit.value)
		ok()
	})
}

// 待たずに取ってくる
function mem_pull(){
	var xmlhttp = new XMLHttpRequest()
	xmlhttp.onload = function(){
		var res = xmlhttp.responseText // 受信した文字列
		updateEdit(res)
		updateLineNum()
	}
	xmlhttp.open("GET", "mem/pull", true)
	xmlhttp.send("pull")
}

//
function mem_wait(){
	var xmlhttp = new XMLHttpRequest()
	xmlhttp.onload = function(){
		if(editor != user){
			var res = xmlhttp.responseText // 受信した文字列
			updateEdit(res)
			console.log('mem_wait')
		}
		mem_wait()
	}
	xmlhttp.open("GET", "mem/wait", true)
	xmlhttp.send("")
}

var users = document.getElementById("users")

// ユーザー一覧をリアルタイム更新
function users_wait(){
	var xmlhttp = new XMLHttpRequest()
	xmlhttp.onload = function(){
		var res = xmlhttp.responseText // 受信した文字列
		users.innerHTML = res
		users_wait()
	}
	xmlhttp.open("GET", "users/wait", true)
	xmlhttp.send("")
}

function editable(bool){
	edit.readOnly = !bool
	assign_button.disabled = bool
	if(bool){
		edit.style.backgroundColor = "#FFD"
	}else{
		edit.style.backgroundColor = "#efefef"
	}
}