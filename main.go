package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/julienschmidt/httprouter"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

type Search struct {
	mx     sync.RWMutex
	Search string
	Length byte
	Links
}

func (s *Search) Mark() {
	s.mx.Lock()
	defer s.mx.Unlock()

	s.Length = (*s).Length - 1
}

func (s *Search) GetLength() byte {
	s.mx.RLock()
	defer s.mx.RUnlock()

	return s.Length
}

const (
	port = "8080"
)

var (
	moscow *time.Location
)

// объекты и методы для хранения паролей
type Links struct {
	mx       sync.RWMutex
	URLs     []string
	Finished bool
}

// Finish finished processing the dataset
func (c *Links) Finish() {
	c.mx.Lock()
	defer c.mx.Unlock()

	// замок
	c.Finished = true
}

// Unfinished returns Finished status
func (c *Links) Unfinished() bool {
	c.mx.RLock()
	defer c.mx.RUnlock()

	// замок
	return !c.Finished
}

func NewURLs() *Links {
	return &Links{
		URLs:     make([]string, 0),
		Finished: false,
	}
}

func Remove(s []string, key int) []string {
	return append(s[:key], s[key+1:]...)
}

// Delete an URL form the map
func (c *Links) DeleteURL(URL string) {
	c.mx.Lock()
	defer c.mx.Unlock()

	// удаляем по значению
	for key, value := range c.URLs {
		if value == URL {
			c.URLs = Remove((*c).URLs, key)
			return
		}
	}
}

// Print prints the URLs
func (c *Links) Print(w http.ResponseWriter) error {
	c.mx.RLock()
	defer c.mx.RUnlock()

	// добавляем URL
	for _, value := range c.URLs {
		_, err := fmt.Fprintln(w, value)
		if err != nil {
			http.Error(w, http.StatusText(503), 503)
			log.Println(err)
			return err
		}
	}

	return nil
}

// GetMap returns the URLs
func (c *Links) GetMap() []string {
	c.mx.RLock()
	defer c.mx.RUnlock()

	return c.URLs
}

// EraseData removes all the data from the map
func (c *Links) EraseUserData() {
	c.mx.Lock()
	defer c.mx.Unlock()

	c.URLs = make([]string, 0)
}

func ProcessURL(URL string, s *Search) {
	fmt.Println("====== Проверяем URL", URL)
	req, err := http.Get(URL)

	if err != nil {
		s.Links.DeleteURL(URL)
		log.Println(err)
	} else {

		var pageData []byte
		pageData, err = ioutil.ReadAll(req.Body)
		// проверяем ещё раз
		if !strings.Contains(string(pageData), (*s).Search) {
			s.Links.DeleteURL(URL)
			fmt.Println("Поисковая строка не обнаружена")
		} else {
			// ничего не делаем
			fmt.Println("Поисковая строка обнаружена! Хорошо!")
		}
	}

	// уменьшаем значение, 0 значит все URL обработаны
	s.Mark()

	// фиксируем что завершена обработка dataSet
	if s.GetLength() == 0 {
		s.Finish()
	}
}

func check(s *Search) {

	// безопасно вычитываем карту
	tempSlice := s.Links.GetMap()

	// проходимся по ней
	for _, URL := range tempSlice {
		go ProcessURL(URL, s)
	}
}

// Middleware wraps julien's router http methods
type Middleware struct {
	router *httprouter.Router
}

// newMiddleware returns pointer of Middleware
func newMiddleware(r *httprouter.Router) *Middleware {
	return &Middleware{r}
}

func main() {

	var err error

	// Устанавливаем сдвиг времени
	moscow, _ = time.LoadLocation("Europe/Moscow")

	var queries Queries

	// объявляем роутер
	var router *Middleware
	router = newMiddleware(
		httprouter.New(),
	)

	// анонсируем хандлеры
	router.router.GET("/", Welcome(&queries))
	router.router.GET("/:action", Welcome(&queries))
	router.router.POST("/", GiveMeURL(&queries))

	webServer := http.Server{
		Addr:              net.JoinHostPort("", port),
		Handler:           router,
		ReadTimeout:       1 * time.Minute,
		ReadHeaderTimeout: 15 * time.Second,
		WriteTimeout:      1 * time.Minute,
	}

	fmt.Println("Launching the service on the port:", port, "...")
	go func() {
		err = webServer.ListenAndServe()
		if err != nil {
			log.Println(err)
		}
	}()

	fmt.Println("The server was launched!")

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt, syscall.SIGTERM)

	<-interrupt

	timeout, cancelFunc := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelFunc()

	err = webServer.Shutdown(timeout)
	if err != nil {
		log.Println(err)
	}

}

// мидлвейр для всех хэндлеров
func (rw *Middleware) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	fmt.Println("-------------------", time.Now().In(moscow).Format(http.TimeFormat), "A request is received -------------------")
	fmt.Println("The request is from", r.RemoteAddr, "| Method:", r.Method, "| URI:", r.URL.String())

	if r.Method == "POST" {
		// проверяем размер POST данных
		r.Body = http.MaxBytesReader(w, r.Body, 1000)
		err := r.ParseForm()
		if err != nil {
			fmt.Println("POST data is exceeded the limit")
			http.Error(w, http.StatusText(400), 400)
			return
		}
	}

	_, err := fmt.Fprintln(w, "<html lang=ru><head><meta charset=UTF-8></head><body>")
	if err != nil {
		log.Println(err)
	}

	rw.router.ServeHTTP(w, r)

	_, err = fmt.Fprint(w, "</body></html>")
	if err != nil {
		log.Println(err)
	}

}

// GiveMeURL processes finished query dataSet
func GiveMeURL(q *Queries) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		var err error

		var searchData string

		fmt.Println("Начинаем обрабатывать данные...<br>")

		searchData = r.PostForm.Get("query")

		if searchData == "" {
			fmt.Println("<b>Ошибка: Запрос не может быть пустым!</b><br>")
			http.Error(w, http.StatusText(400), 400)
			return
		}

		fmt.Println("Данные приняты...<br>")

		var decodedSearchData Search
		err = json.Unmarshal([]byte(searchData), &decodedSearchData)

		// прописываем кол-во URL
		decodedSearchData.Length = byte(len(decodedSearchData.URLs))

		if decodedSearchData.Search == "" {
			fmt.Println("<b>Ошибка: Поисковая фраза не может быть пустой!</b><br>")
			http.Error(w, http.StatusText(400), 400)
			return
		}

		fmt.Println("Поисковая строка:", decodedSearchData.Search, "<br>")

		var dataSet = NewURLs()

		dataSet.URLs = decodedSearchData.URLs

		// запускаем обработку
		check(&decodedSearchData)

		_, err = fmt.Fprintln(w, "<br>Обработка запущена...")
		if err != nil {
			log.Println(err)
		}

		// добавляем ссылку в массив данных запросов
		q.Add(&decodedSearchData)

		_, err = fmt.Fprintln(w, "<a href=/check>Просмотр результатов</a>")
		if err != nil {
			log.Println(err)
		}
	}
}

// Welcome is the homepage of the service
func Welcome(queries *Queries) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, actions httprouter.Params) {
		var err error
		switch {
		case actions.ByName("action") == "check":

			_, err = fmt.Fprintln(w, "Результаты:<br>")
			if err != nil {
				log.Println(err)
			}

			// печатаем список запросов
			queries.List(w)

			_, err = fmt.Fprintln(w, "<br><br><a href=/>Новый запрос</a>")
			if err != nil {
				log.Println(err)
			}
		case r.Method == "GET":
			_, err = fmt.Fprint(w, `<form action="/" method="post">
						<label for="query">Поисковая строка:</label>
						<textarea rows="10" cols="45" name="query" id="query" placeholder="Лалала"></textarea>
						<input type="submit" value="Искать">
					</form>`)
			_, err = fmt.Fprintln(w, "<a href=/check>Просмотр текущих результатов (если ранее запускали)</a>")
			if err != nil {
				http.Error(w, http.StatusText(503), 503)
				log.Println(err)
			}
		}
	}
}
