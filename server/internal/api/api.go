package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"sync"

	"github.com/CassianoJunior/go-react/internal/store/pgstore"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5"
)

type apiHandler struct {
	queries     *pgstore.Queries
	router      *chi.Mux
	upgrader    websocket.Upgrader
	subscribers map[string]map[*websocket.Conn]context.CancelFunc
	mutex       *sync.Mutex
}

func (handler apiHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	handler.router.ServeHTTP(writer, request)
}

func NewHandler(q *pgstore.Queries) http.Handler {
	api := apiHandler{
		queries: q,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
		subscribers: make(map[string]map[*websocket.Conn]context.CancelFunc),
		mutex:       &sync.Mutex{},
	}

	router := chi.NewRouter()

	router.Use(middleware.RequestID, middleware.Logger, middleware.Recoverer)

	router.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"https://*", "http://*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "PATCH"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	router.Get("/subscribe/{room_id}", api.handleSubscribe)

	router.Get("/health", api.Health)

	router.Route("/api", func(router chi.Router) {
		router.Route("/rooms", func(router chi.Router) {
			router.Post("/", api.handleCreateRoom)
			router.Get("/", api.handleFetchRooms)

			router.Route("/{room_id}/messages", func(router chi.Router) {
				router.Get("/", api.handleFetchRoomMessages)
				router.Post("/", api.handleCreateRoomMessage)

				router.Route("/{message_id}", func(router chi.Router) {
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

const (
	KindMessageCreated = "message_created"
)

type MessageCreatedValue struct {
	Id      string `json:"id"`
	Message string `json:"message"`
}

type Message struct {
	RoomId string `json:"-"`
	Kind   string `json:"kind"`
	Value  any    `json:"value"`
}

func (handler apiHandler) notifyClients(message Message) {
	handler.mutex.Lock()
	defer handler.mutex.Unlock()

	subscribers, ok := handler.subscribers[message.RoomId]

	if !ok || len(subscribers) == 0 {
		slog.Info("No subscribers to notify", "room_id", message.RoomId)
		return
	}

	for conn, cancel := range subscribers {
		if err := conn.WriteJSON(message); err != nil {
			slog.Error("Failed to send message", "error", err.Error())
			cancel()
		}
	}
}

func (handler apiHandler) handleSubscribe(writer http.ResponseWriter, request *http.Request) {
	rawRoomId := chi.URLParam(request, "room_id")
	roomId, err := uuid.Parse(rawRoomId)

	if err != nil {
		http.Error(writer, "Invalid room id", http.StatusBadRequest)
		return
	}

	_, err = handler.queries.GetRoom(request.Context(), roomId)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(writer, "room not found", http.StatusNotFound)
			return
		}

		http.Error(writer, "Something went wrong", http.StatusInternalServerError)
		return
	}

	conn, err := handler.upgrader.Upgrade(writer, request, nil)

	if err != nil {
		slog.Warn("Failed to upgrade connection", "error", err.Error())
		http.Error(writer, "Failed to upgrade connection", http.StatusBadRequest)
		return
	}

	defer conn.Close()

	ctx, cancel := context.WithCancel(request.Context())

	handler.mutex.Lock()

	if _, ok := handler.subscribers[rawRoomId]; !ok {
		handler.subscribers[rawRoomId] = make(map[*websocket.Conn]context.CancelFunc)
	}

	slog.Info("New subscriber", "room_id", rawRoomId, "conn", conn.RemoteAddr().String())

	handler.subscribers[rawRoomId][conn] = cancel

	handler.mutex.Unlock()

	<-ctx.Done()

	handler.mutex.Lock()

	delete(handler.subscribers[rawRoomId], conn)

	handler.mutex.Unlock()
}

func (handler apiHandler) Health(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Content-Type", "application/json")
	healthMessage := struct {
		Status string `json:"status"`
	}{
		Status: "ok",
	}

	response, _ := json.Marshal(healthMessage)

	_, _ = writer.Write(response)
}

func (handler apiHandler) handleCreateRoom(writer http.ResponseWriter, request *http.Request) {
	type bodySchema struct {
		Theme string `json:"theme"`
	}
	var body bodySchema
	if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
		http.Error(writer, "Invalid request body", http.StatusBadRequest)
		return
	}

	roomId, err := handler.queries.InsertRoom(request.Context(), body.Theme)

	if err != nil {
		slog.Warn("Failed to create room", "error", err.Error())
		http.Error(writer, "Something went wrong", http.StatusInternalServerError)
		return
	}

	type ResponseSchema struct {
		RoomId string `json:"room_id"`
	}

	response, _ := json.Marshal(ResponseSchema{RoomId: roomId.String()})
	writer.Header().Set("Content-Type", "application/json")
	_, _ = writer.Write(response)
}

func (handler apiHandler) handleFetchRooms(writer http.ResponseWriter, request *http.Request) {}

func (handler apiHandler) handleFetchRoomMessages(writer http.ResponseWriter, request *http.Request) {
}

func (handler apiHandler) handleCreateRoomMessage(writer http.ResponseWriter, request *http.Request) {
	rawRoomId := chi.URLParam(request, "room_id")
	roomId, err := uuid.Parse(rawRoomId)

	if err != nil {
		http.Error(writer, "Invalid room id", http.StatusBadRequest)
		return
	}

	_, err = handler.queries.GetRoom(request.Context(), roomId)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(writer, "room not found", http.StatusNotFound)
			return
		}

		http.Error(writer, "Something went wrong", http.StatusInternalServerError)
		return
	}

	type bodySchema struct {
		Message string `json:"message"`
	}
	var body bodySchema
	if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
		http.Error(writer, "Invalid request body", http.StatusBadRequest)
		return
	}

	messageId, err := handler.queries.InsertMessage(request.Context(), pgstore.InsertMessageParams{RoomID: roomId, Message: body.Message})

	if err != nil {
		slog.Warn("Failed to create message", "error", err.Error())
		http.Error(writer, "Something went wrong", http.StatusInternalServerError)
		return
	}

	type ResponseSchema struct {
		MessageId string `json:"message_id"`
	}

	response, _ := json.Marshal(ResponseSchema{MessageId: messageId.String()})
	writer.Header().Set("Content-Type", "application/json")
	_, _ = writer.Write(response)

	go handler.notifyClients(Message{
		Kind:   KindMessageCreated,
		RoomId: rawRoomId,
		Value: MessageCreatedValue{
			Id:      messageId.String(),
			Message: body.Message,
		},
	})
}

func (handler apiHandler) handleGetRoomMessage(writer http.ResponseWriter, request *http.Request) {}

func (handler apiHandler) handleAddMessageReaction(writer http.ResponseWriter, request *http.Request) {
}

func (handler apiHandler) handleRemoveMessageReaction(writer http.ResponseWriter, request *http.Request) {
}

func (handler apiHandler) handleAnswerMessage(writer http.ResponseWriter, request *http.Request) {}
