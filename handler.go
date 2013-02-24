package exportweibo

import (
	"appengine"
	"appengine/datastore"
	"appengine/taskqueue"
	"appengine/urlfetch"
	"encoding/json"
	"fmt"
	"html/template"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"archive/zip"
	"bytes"
	"io"
)

const (
//	HostName   = "127.0.0.1"
	ApiHost    = ""
	APP_KEY    = "1675182590"
	APP_SECRET = "4879a47fc74e47cb5c8c7643f2e107ad"
)

var PageTemplates *template.Template

type LoginData struct {
	Access_token string
	Remind_in    string
	Expires_in   int
	Uid          string
}

type StatusDS struct {
	Uid    string
	Id     int64
	Text   string
	Status []byte
}

type Status struct {
	Created_at              string
	Id                      int64
	Mid                     string
	Idstr                   string
	Text                    string
	Source                  string
	Favorited               bool
	Truncated               bool
	In_reply_to_status_id   string
	In_reply_to_user_id     string
	In_reply_to_screen_name string
	Thumbnail_pic           string
	Bmiddle_pic             string
	Original_pic            string
	Reposts_count           int
	Comments_count          int
	Attitudes_count         int
	Mlevel                  int
	Retweeted_status        *Status
}

type User struct {
	Id	int64
	Idstr string
	Screen_name string
	Name string
	Province string
	City string
	Location string
	Description string
	Url string
	Profile_image_url string
	Profile_url	string
	Domain string
	Weihao string
	Gender string
	Followers_count int
	Friends_count int
	Statuses_count int
	Favourites_count int
	Created_at string
	Following	bool
	Allow_all_act_msg bool
	Geo_enabled	bool
	Verified	bool
	Verified_type	int
	Remark	string
	Allow_all_comment	bool
	Avatar_large	string
	Verified_reason	string
	Follow_me	bool
	Online_status	int
	Bi_followers_count	int
	Lang string
}

type Timeline struct {
	Statuses        []Status
	Previous_cursor int64
	Next_cursor     int64
	Total_number    int64
}

type TaskProgress struct {
	Uid      string
	Page     int
	Finished bool
}

func init() {

	PageTemplates = template.Must(template.ParseFiles(
		"templates/index.html",
		"templates/export.html",
	))

	http.HandleFunc("/", MainHandler)
	http.HandleFunc("/login/", LoginHandler)
	http.HandleFunc("/export/", ExportHandler)
	http.HandleFunc("/task/progress/", CheckProgressHandler)
	http.HandleFunc("/task/fetcher/", FetcherHandler)
	http.HandleFunc("/task/add/", AddExportTaskHandler)
	http.HandleFunc("/download/", DownloadStatusHandler)
}

func MainHandler(w http.ResponseWriter, r *http.Request) {
	oauth_url := "https://api.weibo.com/oauth2/authorize?client_id=" + APP_KEY + "&redirect_uri=" + r.URL.Scheme + "://" + r.URL.Host + "/login/&response_type=code"
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
		"client_id":     {APP_KEY},
		"client_secret": {APP_SECRET},
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {r.URL.Scheme + "://" + r.URL.Host + "/login/"},
	})
	if err != nil {
		fmt.Fprintln(w, err)
		return
	}

	body, err := ioutil.ReadAll(resp.Body)
	defer resp.Body.Close()
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
	http.Redirect(w, r, "/export/?access_token="+loginData.Access_token+"&uid="+loginData.Uid, 307)
	//fmt.Fprintln(w, loginData.Access_token)
}

func ExportHandler(w http.ResponseWriter, r *http.Request) {
	access_token := r.FormValue("access_token")
	uid := r.FormValue("uid")
	if access_token == "" || uid == "" {
		fmt.Fprintln(w, "<html><meta http-equiv=\"refresh\" content=\"3;url=/\"/><body>please login first, redirect to main page after 3 second.</body></html>")
		return
	}

	get_user_url := "https://api.weibo.com/2/users/show.json?access_token=" + access_token + "&uid=" + uid
	c := appengine.NewContext(r)
	client := urlfetch.Client(c)
	resp, err := client.Get(get_user_url)
	if err != nil {
		fmt.Fprintln(w, err)
		return
	}

	body, err := ioutil.ReadAll(resp.Body)
	defer resp.Body.Close()
	if err != nil {
		fmt.Fprintln(w, err)
		return
	}

	var user User
	err = json.Unmarshal(body, &user)
	if err != nil {
		fmt.Fprintln(w, err)
		fmt.Fprintf(w, "%+v\n", user)
		return
	}

	//fmt.Fprintf(w, "%+v\n", user)
	PageTemplates.ExecuteTemplate(w, "export.html", user)
	//	fmt.Fprintf(w, "retweeted:%+v\n", timeline.Statuses[2].Retweeted_status)

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
	page_count := 50
	total_count, _ := strconv.Atoi(total_number)
	pages := (total_count+page_count-1)/page_count + 1
	clearTaskProgress(c, uid)
	for page := 1; page < pages; page++ {
		taskProgress := TaskProgress{
			Uid:      uid,
			Page:     page,
			Finished: false,
		}
		if _, err := datastore.Put(c, datastore.NewIncompleteKey(c, "TaskProgress", nil), &taskProgress); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		t := taskqueue.NewPOSTTask("/task/fetcher/", url.Values{
			"access_token": {access_token},
			"uid":          {uid},
			"page_count":   {strconv.Itoa(page_count)},
			"page":         {strconv.Itoa(page)},
		})
		if _, err := taskqueue.Add(c, t, ""); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	fmt.Fprint(w, "add export task ok")
}

func FetcherHandler(w http.ResponseWriter, r *http.Request) {
	access_token := r.FormValue("access_token")
	uid := r.FormValue("uid")
	page_count := r.FormValue("page_count")
	page := r.FormValue("page")
	//fmt.Printf("a=%s u=%s c=%s p=%s", access_token, uid, page_count, page)
	if page == "" || page_count == "" || uid == "" || access_token == "" {
		fmt.Println("page == \"\"")
		http.Error(w, "page == \"\"", http.StatusInternalServerError)
		return
	}

	get_timeline_url := "https://api.weibo.com/2/statuses/user_timeline.json?access_token=" + access_token + "&count=" + page_count + "&page=" + page
	c := appengine.NewContext(r)
	client := urlfetch.Client(c)
	resp, err := client.Get(get_timeline_url)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	body, err := ioutil.ReadAll(resp.Body)
	defer resp.Body.Close()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	timeline := Timeline{}
	err = json.Unmarshal(body, &timeline)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	for _, status := range timeline.Statuses {
		//check exist
		cq := datastore.NewQuery("StatusDS").Filter("Id =", status.Id)
		cc, _ := cq.Count(c)
		if cc > 0 {
			continue
		}

		status_bytes, _ := json.Marshal(status)
		statusds := StatusDS{
			Uid:    uid,
			Id:     status.Id,
			Text:   status.Text,
			Status: status_bytes,
		}
		if _, err := datastore.Put(c, datastore.NewIncompleteKey(c, "StatusDS", nil), &statusds); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	page_int, _ := strconv.Atoi(page)
	q := datastore.NewQuery("TaskProgress").Filter("Uid =", uid).Filter("Page =", page_int)
	t := q.Run(c)
	var taskProgress TaskProgress
	key, err := t.Next(&taskProgress)
	if err == nil {
		taskProgress.Finished = true
		datastore.Put(c, key, &taskProgress)
	}
	//w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, "fetcher ok!")
}

func CheckProgressHandler(w http.ResponseWriter, r *http.Request) {
	uid := r.FormValue("uid")
	if uid == "" {
		http.Error(w, "no uid", http.StatusNotFound)
		return
	}

	c := appengine.NewContext(r)
	q := datastore.NewQuery("TaskProgress").Filter("Uid =", uid)
	count, err := q.Count(c)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}

	if count == 0 {
		http.Error(w, "no task", http.StatusNotFound)
		return
	}
	finished_count := 0
	for i := q.Run(c);;{
		var taskProgress TaskProgress
		_, err := i.Next(&taskProgress)
		if err == datastore.Done {
			break
		}

		if taskProgress.Finished == true {
			finished_count += 1
		}
	}
	fmt.Fprintf(w, "{\"count\":%d, \"finished\":%d}", count, finished_count)
}

func DownloadStatusHandler(w http.ResponseWriter, r *http.Request) {
	uid := r.FormValue("uid")
	if uid == "" {
		http.Error(w, "no uid", http.StatusNotFound)
		return
	}

	c := appengine.NewContext(r)
	q := datastore.NewQuery("StatusDS").Filter("Uid =", uid).Order("-Id")

	statuses := "var timeline = {\"statuses\":[ "
	count := 0
	for i := q.Run(c);; {
		var statusds StatusDS
		_, err := i.Next(&statusds)
		if err == datastore.Done {
			statuses = statuses[:len(statuses)-1]
			statuses += "],\"count\":" + strconv.Itoa(count) + "};"
			break
		}

		statuses += string(statusds.Status) + ","
		count++
	}
	
	zipbytes, err := makeZipFile(statuses)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Disposition", "attachment; filename=\"weibo.zip\"")
	w.Write(zipbytes)
}

func makeZipFile(statuses string) (zipbytes []byte, err error){
	reader, err := zip.OpenReader("templates/weibo.zip")
	if err != nil {
		return
	}

	defer reader.Close()

	buf := new(bytes.Buffer)
	writer := zip.NewWriter(buf)

	for _, f := range reader.File {
		wf, err := writer.Create(f.Name)
		if err != nil {
			continue
		}

		rf, err := f.Open()
		if err != nil {
			continue
		}

		io.Copy(wf, rf)
		rf.Close()
	}

	wf, err := writer.Create("js/timeline.js")
	if err != nil {
		return
	}
	_, err = wf.Write([]byte(statuses))
	if err != nil {
		return
	}

	err = writer.Close()
	if err != nil {
		return
	}

	return buf.Bytes(), err
}
