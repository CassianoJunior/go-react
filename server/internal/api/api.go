package api

import (
	"net/http"

	"github.com/CassianoJunior/go-react/internal/store/pgstore"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
)

type apiHandler struct {
	q      *pgstore.Queries
	router *chi.Mux
}

func (handler apiHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	handler.router.ServeHTTP(writer, request)
}

func NewHandler(q *pgstore.Queries) http.Handler {
	api := apiHandler{
		q: q,
	}

	router := chi.NewRouter()

	router.Use(middleware.RequestID, middleware.Logger, middleware.Recoverer)

	router.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"https://*", "http://*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "PATCH"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: false,
		MaxAge:           300, // Maximum value not ignored by any of major browsers
	}))

	router.Get("/subscribe/{room_id}", api.handleSubscribe)

	router.Route("/api", func(r chi.Router) {
		router.Get("/", api.Health)
		router.Route("rooms", func(r chi.Router) {
			router.Post("/", api.handleCreateRoom)
			router.Get("/", api.handleFetchRooms)

			router.Route("/{room_id}/messages", func(r chi.Router) {
				router.Get("/", api.handleFetchRoomMessages)
				router.Post("/", api.handleCreateRoomMessage)

				router.Route("/{message_id}", func(r chi.Router) {
					router.Get("/", api.handleGetRoomMessage)
					router.Patch("/reaction", api.handleAddMessageReaction)
					router.Delete("/reaction", api.handleRemoveMessageReaction)
					router.Patch("/answer", api.handleAnswerMessage)
				})
			})
		})
	})

	api.router = router
	return api
}

func (handler apiHandler) handleSubscribe(writer http.ResponseWriter, request *http.Request) {}

func (handler apiHandler) Health(writer http.ResponseWriter, request *http.Request) {}

func (handler apiHandler) handleCreateRoom(writer http.ResponseWriter, request *http.Request) {}

func (handler apiHandler) handleFetchRooms(writer http.ResponseWriter, request *http.Request) {}

func (handler apiHandler) handleFetchRoomMessages(writer http.ResponseWriter, request *http.Request) {
}

func (handler apiHandler) handleCreateRoomMessage(writer http.ResponseWriter, request *http.Request) {
}

func (handler apiHandler) handleGetRoomMessage(writer http.ResponseWriter, request *http.Request) {}

func (handler apiHandler) handleAddMessageReaction(writer http.ResponseWriter, request *http.Request) {
}

func (handler apiHandler) handleRemoveMessageReaction(writer http.ResponseWriter, request *http.Request) {
}

func (handler apiHandler) handleAnswerMessage(writer http.ResponseWriter, request *http.Request) {}
