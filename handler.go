package exportweibo

import (
	"fmt"
	"net/http"
	"net/url"
	"html/template"
	"io/ioutil"
	"strconv"
	"appengine"
	"appengine/urlfetch"
	"appengine/taskqueue"
	"appengine/datastore"
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

type TaskProgress struct {
	Uid string
	Page int
	Finished bool
}

func init() {

	PageTemplates = template.Must(template.ParseFiles(
					"templates/index.html",
					"templates/login.html",
				))

	http.HandleFunc("/", MainHandler)
	http.HandleFunc("/login/", LoginHandler)
	http.HandleFunc("/export/", ExportHandler)
	http.HandleFunc("/progress/", CheckProgressHandler)
	http.HandleFunc("/task/fetcher/", FetcherHandler)
	http.HandleFunc("/task/add/", AddExportTaskHandler)
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

func CheckProgressHandler(w http.ResponseWriter, r *http.Request) {
}

func clearTaskProgress(c appengine.Context, uid string) {
	q := datastore.NewQuery("TaskProgress").Filter("Uid =", uid)
	tmp := []TaskProgress{}
	if keys, err := q.GetAll(c, &tmp); err == nil {
		datastore.DeleteMulti(c, keys)
	}
}

func AddExportTaskHandler(w http.ResponseWriter, r *http.Request) {
	access_token := r.FormValue("access_token")
	uid := r.FormValue("uid")
	total_number := r.FormValue("total_number")
	if access_token == "" || uid == "" || total_number == "" {
		fmt.Fprintln(w, "<html><meta http-equiv=\"refresh\" content=\"3;url=/\"/><body>please login first, redirect to main page after 3 second.</body></html>")
		return
	}

	c := appengine.NewContext(r)
	page_count := 5000
	total_count, _ := strconv.Atoi(total_number)
	pages := (total_count + page_count - 1) / page_count + 1
	clearTaskProgress(c, uid)
	for page:= 1 ; page < pages ; page++ {

		taskProgress := TaskProgress{
			Uid: uid,
			Page: page,
			Finished: false,
		}
		if _, err := datastore.Put(c, datastore.NewIncompleteKey(c, "TaskProgress", nil), &taskProgress); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}


		t := taskqueue.NewPOSTTask("/task/fetcher/", url.Values{
									"access_token": {access_token},
									"uid": {uid},
									"page_count": {string(page_count)},
									"page": {string(page)},
								})
		if _, err := taskqueue.Add(c, t, ""); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}

func FetcherHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, "ok")
}

