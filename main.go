package main

import (
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type Player struct {
	ID          int
	Conn        *websocket.Conn
	PositionY   float64 // Vertical position
	PositionX   float64
	Velocity    float64 // Vertical velocity
	Alive       bool
	DashReady   bool
	ObstacleReady bool
}

type Obstacle struct {
	PositionX float64
}

var (
	upgrader       = websocket.Upgrader{}
	players        = make(map[int]*Player)
	playerID       = 0
	playersMux     sync.Mutex
	obstacles      []Obstacle
	obstacleSpeed  = 5.0
	cooldownPeriod = 10 * time.Second
)

func main() {
	rand.Seed(time.Now().UnixNano())
	http.HandleFunc("/ws", handleConnections)
	go gameLoop()

	fmt.Println("Server started on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func handleConnections(w http.ResponseWriter, r *http.Request) {
	upgrader.CheckOrigin = func(r *http.Request) bool { return true }
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Upgrade error:", err)
		return
	}
	defer conn.Close()

	playersMux.Lock()
	playerID++
	id := playerID
	player := &Player{
		ID:          id,
		Conn:        conn,
		PositionY:   0,
		PositionX:   500,
		Velocity:    15,
		Alive:       true,
		DashReady:   true,
		ObstacleReady: true,
	}
	players[id] = player
	playersMux.Unlock()

	defer func() {
		playersMux.Lock()
		delete(players, id)
		playersMux.Unlock()
	}()

	fmt.Printf("Player %d connected\n", id)

	// Send the player their ID
	conn.WriteJSON(map[string]interface{}{
		"type": "init",
		"id":   id,
	})

	for {
		var msg map[string]interface{}
		err := conn.ReadJSON(&msg)
		if err != nil {
			log.Println("Read error:", err)
			break
		}

		if !player.Alive {
			continue
		}

		switch msg["type"] {
		case "left":
			playersMux.Lock()
			if player.PositionX > 0 {
				player.PositionX -= 20
			}
			playersMux.Unlock()
		case "right":
			playersMux.Lock()
			if player.PositionX < 1000 {
				player.PositionX += 20
			}
			playersMux.Unlock()
		case "jump":
			playersMux.Lock()
			if player.PositionY == 0 {
				player.Velocity = 25 
			}
			playersMux.Unlock()
		case "dash":
			if player.DashReady {
				playersMux.Lock()
				player.PositionX += 200 
				player.DashReady = false
				playersMux.Unlock()
				go resetDashCooldown(player)
			}
		case "moveObstacle":
			if player.ObstacleReady {
				playersMux.Lock()
				for i := range obstacles {
					obstacles[i].PositionX += 200 
				}
				player.ObstacleReady = false
				playersMux.Unlock()
				go resetObstacleCooldown(player)
			}
		case "restart":
			playersMux.Lock()
			player.Alive = true
			player.PositionY = 0
			player.PositionX = 500
			player.Velocity = 15
			player.DashReady = true
			player.ObstacleReady = true
			playersMux.Unlock()
		}
	}
}

func resetDashCooldown(player *Player) {
	time.Sleep(cooldownPeriod)
	playersMux.Lock()
	player.DashReady = true
	playersMux.Unlock()
}

func resetObstacleCooldown(player *Player) {
	time.Sleep(cooldownPeriod)
	playersMux.Lock()
	player.ObstacleReady = true
	playersMux.Unlock()
}

func gameLoop() {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	obstacleTimer := time.NewTimer(randomObstacleInterval())

	for {
		select {
		case <-ticker.C:
			updateGameState()
			broadcastGameState()
		case <-obstacleTimer.C:
			spawnObstacle()
			obstacleTimer.Reset(randomObstacleInterval())
		}
	}
}

func updateGameState() {
	playersMux.Lock()
	defer playersMux.Unlock()

	for _, player := range players {
		if !player.Alive {
			continue
		}

		player.PositionY += player.Velocity
		player.Velocity -= 1.0 


		if player.PositionY < 0 {
			player.PositionY = 0
			player.Velocity = 0
		}

		for _, obs := range obstacles {
			if obs.PositionX >= player.PositionX && obs.PositionX <= player.PositionX+50 && player.PositionY <= 50 {
				player.Conn.WriteJSON(map[string]interface{}{
					"type": "gameover",
				})
				player.Alive = false
			}
		}
	}

	for i := 0; i < len(obstacles); i++ {
		obstacles[i].PositionX -= obstacleSpeed
		if obstacles[i].PositionX < -20 {
			obstacles = append(obstacles[:i], obstacles[i+1:]...)
			i--
		}
	}
}

func broadcastGameState() {
	playersMux.Lock()
	defer playersMux.Unlock()

	state := map[string]interface{}{
		"type":      "state",
		"players":   []map[string]interface{}{},
		"obstacles": []map[string]interface{}{},
	}

	for _, player := range players {
		state["players"] = append(state["players"].([]map[string]interface{}), map[string]interface{}{
			"id":        player.ID,
			"positionX": player.PositionX,
			"positionY": player.PositionY,
			"alive":     player.Alive,
		})
	}

	for _, obs := range obstacles {
		state["obstacles"] = append(state["obstacles"].([]map[string]interface{}), map[string]interface{}{
			"positionX": obs.PositionX,
		})
	}

	for _, player := range players {
		err := player.Conn.WriteJSON(state)
		if err != nil {
			log.Println("Write error:", err)
			player.Conn.Close()
			delete(players, player.ID)
		}
	}
}

func spawnObstacle() {
	obstacles = append(obstacles, Obstacle{
		PositionX: 2000, 
	})
}

func randomObstacleInterval() time.Duration {
	return time.Duration(rand.Intn(2000)+1000) * time.Millisecond
}
