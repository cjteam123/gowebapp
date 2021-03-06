package session

import (
	"crypto/rand"
	"encoding/base64"
	"log"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// Manager is a singleton that starts and ends sessions
type Manager struct {
	lock     sync.Mutex
	sessions map[string]*Session
}

// Time until session is destroyed
const lifespan = 2 * time.Hour

// Cookie name to save the session ID with
const cookieName = "go-webapp-sessionid"

// Frequency the sessions are cleared
const cleanFrequency = 10 * time.Minute

// Singleton storage
var sessionManager = Manager{
	sessions: make(map[string]*Session),
}

// Run a seperate thread to periodically clean the unused sessions
func init() {
	go cleanSessions()
}

// GetManager returns the session manager, which can be used to start and end sessions
func GetManager() *Manager {
	return &sessionManager
}

// Start is a method that returns the session associated with the current user. If there is
// not yet a session, create a new one.
func (manager *Manager) Start(responseWriter http.ResponseWriter, request *http.Request) *Session {
	manager.lock.Lock()
	defer manager.lock.Unlock()

	// Check if the client has a cookie with a session ID
	cookie, err := request.Cookie(cookieName)
	var sessionID string
	if err != nil || cookie.Value == "" {
		// Create a session ID to keep track of the session
		sessionID = createSessionID()
	} else {
		sessionID, _ = url.QueryUnescape(cookie.Value)
		// Use the client's session ID to get their session
		// If session doesn't exist, drop through
		if session, exists := manager.sessions[sessionID]; exists {
			session.lastUsed = time.Now()
			return session
		}
	}

	// Create the new session
	session := NewSession(sessionID)
	manager.sessions[sessionID] = session
	// Store the session in the client's cookies
	http.SetCookie(responseWriter, &http.Cookie{
		Name:     cookieName,
		Value:    url.QueryEscape(sessionID),
		Path:     "/",
		HttpOnly: true,
		MaxAge:   int(lifespan / time.Second),
	})
	return session
}

// End is a method that ends the session associated with the current user. If there is no session,
// do nothing.
func (manager *Manager) End(responseWriter http.ResponseWriter, request *http.Request) {
	cookie, err := request.Cookie(cookieName)

	// Client has no session, ignore
	if err != nil || cookie.Value == "" {
		return
	}

	// Destroy the session and the cookie
	manager.lock.Lock()
	defer manager.lock.Unlock()
	delete(manager.sessions, cookie.Value)
	http.SetCookie(responseWriter, &http.Cookie{
		Name:     cookieName,
		Path:     "/",
		HttpOnly: true,
		Expires:  time.Now(),
		MaxAge:   -1,
	})
}

// createSessionID gnerates a crytographically secure session ID
func createSessionID() string {
	token := make([]byte, 32)
	if _, err := rand.Read(token); err != nil {
		log.Fatal(err)
		return ""
	}
	return base64.URLEncoding.EncodeToString(token)
}

// cleanSessions is an asynchronous function to remove old sessions
func cleanSessions() {
	for {
		// Use a channel to sleep the thread until the time duration is complete
		<-time.After(cleanFrequency)

		sessionManager.lock.Lock()
		// Iterate through the map and remove entries that have expired
		for id, session := range sessionManager.sessions {
			if time.Now().After(session.lastUsed.Add(lifespan)) {
				delete(sessionManager.sessions, id)
			}
		}
		sessionManager.lock.Unlock()
	}
}
