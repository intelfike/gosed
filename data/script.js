var editor = ""
var user = ""
// 初期化
const init = ()=>{
	user = localStorage.getItem('user')
	var file = document.cookie.match(/file=[^\s;]+/g)
	if(file.length != 1){
		alert("ファイル指定のcookieが正しくありません")
		return
	}
	document.title = file[0].substring(5)
	
	editable(false)
	users_wait()
	assign_wait()
	mem_pull()
	mem_wait()
	edit.onclick()
	edit.focus()
	updateLineNum()
}

document.body.onload = init

var assign_button = document.getElementById('assign')
assign_button.onclick = ()=>{
	// 自分自身に権限を割り当てる必要がある
	assign_send(user)
}

var edit = document.getElementById("edit")
async function save(){
	await mem_send()
	await ajax('POST', '/save')
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
var prevlen = 0
function updateLineNum(){
	len = (edit.value.match(/\n/g)||[]).length+1
	if(prevlen == len){
		return
	}
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
	prevlen = len
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
	ajax("POST", "/user/assign/push", user_name)
}
async function assign_wait(){	
	var res = await ajax("GET", "/user/assign/wait")
	console.log("=======",res, "===", user)
	editor = res
	editable(editor == user)
	check_user(editor)
	assign_wait()
}

// 送ってメモリ上に保存・共有する
function mem_send(){
	ajax("POST", "/mem/push", edit.value)
}

// 待たずに取ってくる
async function mem_pull(){
	var res = await ajax("GET", "/mem/pull", 'pull')
	updateEdit(res)
	updateLineNum()
}

async function mem_wait(){
	var res = await ajax("GET", "/mem/wait", 'pull')
	if(editor != user){
		updateEdit(res)
		console.log('mem_wait')
	}
	mem_wait()
}

var users = document.getElementById("users")

// ユーザー一覧をリアルタイム更新
async function users_wait(){
	var res = await ajax("GET", "/users/wait", 'pull')
	users.innerHTML = res
	users_wait()
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

function check_user(name){
	document.getElementById(name).checked = true
}

function ajax(method, action, data){
	return new Promise(ok => {
		var aj = new XMLHttpRequest()
		aj.open(method, action)
		aj.onload = ()=>{
			 ok(aj.responseText)
		}
		aj.send(data)
	})
}