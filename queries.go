package main

import (
	"fmt"
	"log"
	"net/http"
	"sync"
)

type Queries struct {
	mx       sync.RWMutex
	set []*Search
}

func (q *Queries) Add(s *Search) {
	q.mx.Lock()
	defer q.mx.Unlock()

	q.set = append((*q).set, s)
}

func (q *Queries) List(w http.ResponseWriter) {
	q.mx.RLock()
	defer q.mx.RUnlock()

	// Чертим список запросов
	if q.set == nil {
		fmt.Println("Запросов пока небыло")
		return
	}

	fmt.Println("Найдено: ", len((*q).set), "<br>")
	for key, value := range (*q).set {
		_, err := fmt.Fprintln(w, "Запрос строки ", (*value).Search, ". Кол-во URL:", len((*value).URLs), ". <a=/url/",key,">Смотреть результаты</a><br>")
		if err != nil {
			http.Error(w, http.StatusText(503), 503)
			log.Println(err)
		}
	}
}