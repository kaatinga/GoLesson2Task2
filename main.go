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
	"strconv"
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

type TmpSearch struct {
	Search string
	URLs   []string
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

func (s *Search) check(t *TmpSearch) {

	// проходимся по срезу с URL
	for _, URL := range (*t).URLs {
		fmt.Println("Запускаем горутину для проверки URL:", URL, ". Строка поиска: ", t.Search)
		go ProcessURL(URL, s)
	}
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

// AddURL adds an URL to the slice
func (c *Links) AddURL(URL string) {
	c.mx.Lock()
	defer c.mx.Unlock()

	c.URLs = append((*c).URLs, URL)
}

func ProcessURL(URL string, s *Search) {
	fmt.Println("====== Проверяем URL", URL)
	req, err := http.Get(URL)
	if err != nil {
		log.Println(err)
	} else {

		var pageData []byte
		pageData, err = ioutil.ReadAll(req.Body)
		// проверяем ещё раз
		if !strings.Contains(string(pageData), (*s).Search) {
			fmt.Println("====== По адресу ", URL, " поисковая строка не обнаружена:")
		} else {
			s.Links.AddURL(URL)
			fmt.Println("====== По адресу ", URL, " поисковая строка обнаружена! Хорошо!")
		}
	}

	// уменьшаем значение, 0 значит все URL обработаны
	s.Mark()

	// фиксируем что завершена обработка dataSet
	if s.GetLength() == 0 {
		s.Finish()
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
	router.router.GET("/result/:action", Welcome(&queries))
	router.router.POST("/", GiveMeURL(&queries))
	router.router.GET("/url/:id", Result(&queries))

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

	// favicon.ico костыль
	if r.URL.String() == "/favicon.ico" {
		http.ServeFile(w, r, "favicon.ico")
		return
	}

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

	// начало файла
	_, err := fmt.Fprintln(w, "<html lang=ru><head><meta charset=UTF-8></head><body>")
	if err != nil {
		log.Println(err)
	}

	// хэндлеры тут
	rw.router.ServeHTTP(w, r)

	// конец файла
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

		fmt.Println("Начинаем обрабатывать данные...")

		searchData = r.PostForm.Get("query")

		if searchData == "" {
			log.Println("Ошибка: Запрос не может быть пустым!")
			return
		}

		fmt.Println("Данные приняты...<br>")

		var tmpDecodedSearchData TmpSearch

		err = json.Unmarshal([]byte(searchData), &tmpDecodedSearchData)
		if err != nil {
			log.Println(err)
			
			return
		}

		if tmpDecodedSearchData.Search == "" {
			log.Println("Ошибка: Поисковая фраза не может быть пустой!")
			return
		}

		fmt.Println("Поисковая строка:", tmpDecodedSearchData.Search)

		// прописываем строку поиска в финальный dataSet
		var decodedSearchData Search
		decodedSearchData.Search = tmpDecodedSearchData.Search

		// проверяем кол-во URL
		URLNumber := len(tmpDecodedSearchData.URLs)
		if URLNumber == 0 {
			log.Println("Кол-во URL нулевое, неверный запрос!")
			return
		}
		fmt.Println("Кол-во URL в запросе:", tmpDecodedSearchData.Search)

		// прописываем кол-во URL
		decodedSearchData.Length = byte(URLNumber)

		// запускаем обработку (прописываем URL)
		decodedSearchData.check(&tmpDecodedSearchData)

		_, err = fmt.Fprintln(w, "Обработка запущена...<br>")
		if err != nil {
			
			log.Println(err)
		}

		// добавляем ссылку в массив данных запросов
		q.Add(&decodedSearchData)

		_, err = fmt.Fprintln(w, "<br><a href=/result/check>Просмотр результатов</a>")
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

			_, err = fmt.Fprintln(w, "<b>Результаты:</b><br>")
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
			_, err = fmt.Fprintln(w, `<form action="/" method="post">
						<label for="query">Поисковая строка:</label>
						<textarea rows="10" cols="45" name="query" id="query" placeholder="Лалала"></textarea>
						<input type="submit" value="Искать">
					</form>`)
			if err != nil {
				log.Println(err)
			}

			_, err = fmt.Fprintln(w, "<a href=/result/check>Просмотр всех результатов</a>")
			if err != nil {
				log.Println(err)
			}
		}
	}
}

func Result(q *Queries) httprouter.Handle {
	return func(w http.ResponseWriter, _ *http.Request, actions httprouter.Params) {
		queryID := actions.ByName("id")
		id, err := strconv.Atoi(queryID)
		if err != nil {
			log.Println(err)
			return
		}

		q.GetOne(w, byte(id))
	}
}
