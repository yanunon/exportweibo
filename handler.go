package exportweibo

import (
	"fmt"
	"net/http"
	"net/url"
	"html/template"
	"io/ioutil"
	"appengine"
	"appengine/urlfetch"
	"encoding/json"
)

const(
	HostName = "127.0.0.1"
	ApiHost = ""
	APP_KEY = "1675182590"
	APP_SECRET = "4879a47fc74e47cb5c8c7643f2e107ad"
)

var PageTemplates *template.Template

type LoginData struct {
	Access_token string
	Remind_in string
	Expires_in int
	Uid string
}

type Status struct {
	Created_at string
	Id	int64
	Mid	string
	Idstr	string
	Text	string
	Source	string
	Favorited bool
	Truncated bool
	In_reply_to_status_id string
	In_reply_to_user_id string
	In_reply_to_screen_name string
	Thumbnail_pic	string
	Bmiddle_pic	string
	Original_pic	string
	Reposts_count	int
	Comments_count	int
	Attitudes_count	int
	Mlevel	int
	Retweeted_status *Status
}

type Timeline struct {
	Statuses []Status
	Previous_cursor int64
	Next_cursor int64
	Total_number int64
}

func init() {

	PageTemplates = template.Must(template.ParseFiles(
					"templates/index.html",
					"templates/login.html",
				))

	http.HandleFunc("/", MainHandler)
	http.HandleFunc("/login/", LoginHandler)
	http.HandleFunc("/export/", ExportHandler)
	http.HandleFunc("/process/", CheckProcessHandler)
	http.HandleFunc("/task/", TaskHandler)
}

func MainHandler(w http.ResponseWriter, r *http.Request) {
	oauth_url := "https://api.weibo.com/oauth2/authorize?client_id=" + APP_KEY + "&redirect_uri=" + r.URL.Scheme + "http://" + HostName + "/login/&response_type=code"
	PageTemplates.ExecuteTemplate(w, "index.html", oauth_url)
}


func LoginHandler(w http.ResponseWriter, r *http.Request) {
	code := r.FormValue("code")
	if code == "" {
		fmt.Fprintln(w, "must have code")
		return
	}
	access_url := "https://api.weibo.com/oauth2/access_token"
	c := appengine.NewContext(r)
	client := urlfetch.Client(c)
	resp, err := client.PostForm(access_url, url.Values{
						"client_id" : {APP_KEY},
						"client_secret" : {APP_SECRET},
						"grant_type" : {"authorization_code"},
						"code" : {code},
						"redirect_uri" : {"http://127.0.0.1/"},
					})
	if err != nil {
		fmt.Fprintln(w, err)
		return
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Fprintln(w, err)
		return
	}
	loginData := LoginData{}
	err = json.Unmarshal(body, &loginData)
	if err != nil {
		fmt.Fprintf(w, "%+v\n", loginData)
		fmt.Fprintln(w, err)
		return
	}
	http.Redirect(w, r, "/export/?access_token=" + loginData.Access_token + "&uid=" + loginData.Uid, 307)
	//fmt.Fprintln(w, loginData.Access_token)
}

func ExportHandler(w http.ResponseWriter, r *http.Request) {
	access_token := r.FormValue("access_token")
	uid := r.FormValue("uid")
	if access_token == "" || uid == "" {
		fmt.Fprintln(w, "<html><meta http-equiv=\"refresh\" content=\"3;url=/\"/><body>please login first, redirect to main page after 3 second.</body></html>")
		return
	}

	get_status_count_url := "https://api.weibo.com/2/statuses/user_timeline.json?access_token=" + access_token
	c := appengine.NewContext(r)
	client := urlfetch.Client(c)
	resp, err := client.Get(get_status_count_url)
	if err != nil {
		fmt.Fprintln(w, err)
		return
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Fprintln(w, err)
		return
	}
	
	timeline := Timeline{}
	err = json.Unmarshal(body, &timeline)
	if err != nil {
		fmt.Fprintln(w, err)
		fmt.Fprintf(w, "%+v\n", timeline)
		return
	}

	fmt.Fprintf(w, "%+v\n", timeline)
//	fmt.Fprintf(w, "retweeted:%+v\n", timeline.Statuses[2].Retweeted_status)

}

func CheckProcessHandler(w http.ResponseWriter, r *http.Request) {
}

func TaskHandler(w http.ResponseWriter, r *http.Request) {
}

