// Copyright © 2015 Steve Francia <spf@spf13.com>.
//
//   Licensed under the Apache License, Version 2.0 (the "License");
//   you may not use this file except in compliance with the License.
//   You may obtain a copy of the License at
//
//       http://www.apache.org/licenses/LICENSE-2.0
//
//   Unless required by applicable law or agreed to in writing, software
//   distributed under the License is distributed on an "AS IS" BASIS,
//   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//   See the License for the specific language governing permissions and
//   limitations under the License.
//

package commands

import (
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"time"

	"bitbucket.org/mannih/gc6/mazelib"
	"github.com/gin-gonic/gin"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type Maze struct {
	rooms      [][]mazelib.Room
	start      mazelib.Coordinate
	end        mazelib.Coordinate
	icarus     mazelib.Coordinate
	StepsTaken int
}

// Tracking the current maze being solved

// WARNING: This approach is not safe for concurrent use
// This server is only intended to have a single client at a time
// We would need a different and more complex approach if we wanted
// concurrent connections than these simple package variables
var currentMaze *Maze
var scores []int

// Defining the daedalus command.
// This will be called as 'laybrinth daedalus'
var daedalusCmd = &cobra.Command{
	Use:     "daedalus",
	Aliases: []string{"deadalus", "server"},
	Short:   "Start the laybrinth creator",
	Long: `Daedalus's job is to create a challenging Labyrinth for his opponent
  Icarus to solve.

  Daedalus runs a server which Icarus clients can connect to to solve laybrinths.`,
	Run: func(cmd *cobra.Command, args []string) {
		RunServer()
	},
}

func init() {
	rand.Seed(time.Now().UTC().UnixNano()) // need to initialize the seed
	gin.SetMode(gin.ReleaseMode)

	RootCmd.AddCommand(daedalusCmd)
}

// Runs the web server
func RunServer() {
	// Adding handling so that even when ctrl+c is pressed we still print
	// out the results prior to exiting.
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		<-c
		printResults()
		os.Exit(1)
	}()

	// Using gin-gonic/gin to handle our routing
	r := gin.Default()
	v1 := r.Group("/")
	{
		v1.GET("/awake", GetStartingPoint)
		v1.GET("/move/:direction", MoveDirection)
		v1.GET("/done", End)
	}

	r.Run(":" + viper.GetString("port"))
}

// Ends a session and prints the results.
// Called by Icarus when he has reached
//   the number of times he wants to solve the laybrinth.
func End(c *gin.Context) {
	printResults()
	os.Exit(1)
}

// initializes a new maze and places Icarus in his awakening location
func GetStartingPoint(c *gin.Context) {
	initializeMaze()
	startRoom, err := currentMaze.Discover(currentMaze.Icarus())
	if err != nil {
		fmt.Println("Icarus is outside of the maze. This shouldn't ever happen")
		fmt.Println(err)
		os.Exit(-1)
	}
	mazelib.PrintMaze(currentMaze)
	c.JSON(http.StatusOK, mazelib.Reply{Survey: startRoom})
}

// The API response to the /move/:direction address
func MoveDirection(c *gin.Context) {
	var err error

	switch c.Param("direction") {
	case "left":
		err = currentMaze.MoveLeft()
	case "right":
		err = currentMaze.MoveRight()
	case "down":
		err = currentMaze.MoveDown()
	case "up":
		err = currentMaze.MoveUp()
	}

	var r mazelib.Reply

	if err != nil {
		r.Error = true
		r.Message = err.Error()
		c.JSON(409, r)
		return
	}

	s, e := currentMaze.LookAround()

	if e != nil {
		if e == mazelib.ErrVictory {
			scores = append(scores, currentMaze.StepsTaken)
			r.Victory = true
			r.Message = fmt.Sprintf("Victory achieved in %d steps \n", currentMaze.StepsTaken)
		} else {
			r.Error = true
			r.Message = err.Error()
		}
	}
	r.Survey = s
	c.JSON(http.StatusOK, r)
}

func initializeMaze() {
	currentMaze = createMaze()
}

// Print to the terminal the average steps to solution for the current session
func printResults() {
	fmt.Printf("Labyrinth solved %d times with an avg of %d steps\n", len(scores), mazelib.AvgScores(scores))
}

// Return a room from the maze
func (m *Maze) GetRoom(x, y int) (*mazelib.Room, error) {
	if x < 0 || y < 0 || x >= m.Width() || y >= m.Height() {
		return &mazelib.Room{}, errors.New("room outside of maze boundaries")
	}

	return &m.rooms[y][x], nil
}

func (m *Maze) Width() int  { return len(m.rooms[0]) }
func (m *Maze) Height() int { return len(m.rooms) }

// Return Icarus's current position
func (m *Maze) Icarus() (x, y int) {
	return m.icarus.X, m.icarus.Y
}

// Set the location where Icarus will awake
func (m *Maze) SetStartPoint(x, y int) error {
	r, err := m.GetRoom(x, y)

	if err != nil {
		return err
	}

	if r.Treasure {
		return errors.New("can't start in the treasure")
	}

	r.Start = true
	m.icarus = mazelib.Coordinate{x, y}
	return nil
}

// Set the location of the treasure for a given maze
func (m *Maze) SetTreasure(x, y int) error {
	r, err := m.GetRoom(x, y)

	if err != nil {
		return err
	}

	if r.Start {
		return errors.New("can't have the treasure at the start")
	}

	r.Treasure = true
	m.end = mazelib.Coordinate{x, y}
	return nil
}

// Given Icarus's current location, Discover that room
// Will return ErrVictory if Icarus is at the treasure.
func (m *Maze) LookAround() (mazelib.Survey, error) {
	if m.end.X == m.icarus.X && m.end.Y == m.icarus.Y {
		fmt.Printf("Victory achieved in %d steps \n", m.StepsTaken)
		return mazelib.Survey{}, mazelib.ErrVictory
	}

	return m.Discover(m.icarus.X, m.icarus.Y)
}

// Given two points, survey the room.
// Will return error if two points are outside of the maze
func (m *Maze) Discover(x, y int) (mazelib.Survey, error) {
	if r, err := m.GetRoom(x, y); err != nil {
		return mazelib.Survey{}, nil
	} else {
		return r.Walls, nil
	}
}

// Moves Icarus's position left one step
// Will not permit moving through walls or out of the maze
func (m *Maze) MoveLeft() error {
	s, e := m.LookAround()
	if e != nil {
		return e
	}
	if s.Left {
		return errors.New("Can't walk through walls")
	}

	x, y := m.Icarus()
	if _, err := m.GetRoom(x-1, y); err != nil {
		return err
	}

	m.icarus = mazelib.Coordinate{x - 1, y}
	m.StepsTaken++
	return nil
}

// Moves Icarus's position right one step
// Will not permit moving through walls or out of the maze
func (m *Maze) MoveRight() error {
	s, e := m.LookAround()
	if e != nil {
		return e
	}
	if s.Right {
		return errors.New("Can't walk through walls")
	}

	x, y := m.Icarus()
	if _, err := m.GetRoom(x+1, y); err != nil {
		return err
	}

	m.icarus = mazelib.Coordinate{x + 1, y}
	m.StepsTaken++
	return nil
}

// Moves Icarus's position up one step
// Will not permit moving through walls or out of the maze
func (m *Maze) MoveUp() error {
	s, e := m.LookAround()
	if e != nil {
		return e
	}
	if s.Top {
		return errors.New("Can't walk through walls")
	}

	x, y := m.Icarus()
	if _, err := m.GetRoom(x, y-1); err != nil {
		return err
	}

	m.icarus = mazelib.Coordinate{x, y - 1}
	m.StepsTaken++
	return nil
}

// Moves Icarus's position down one step
// Will not permit moving through walls or out of the maze
func (m *Maze) MoveDown() error {
	s, e := m.LookAround()
	if e != nil {
		return e
	}
	if s.Bottom {
		return errors.New("Can't walk through walls")
	}

	x, y := m.Icarus()
	if _, err := m.GetRoom(x, y+1); err != nil {
		return err
	}

	m.icarus = mazelib.Coordinate{x, y + 1}
	m.StepsTaken++
	return nil
}

// Creates a maze without any walls
// Good starting point for additive algorithms
func emptyMaze() *Maze {
	z := Maze{}
	ySize := viper.GetInt("height")
	xSize := viper.GetInt("width")

	z.rooms = make([][]mazelib.Room, ySize)
	for y := 0; y < ySize; y++ {
		z.rooms[y] = make([]mazelib.Room, xSize)
		for x := 0; x < xSize; x++ {
			z.rooms[y][x] = mazelib.Room{}
		}
	}

	return &z
}

// Creates a maze with all walls
// Good starting point for subtractive algorithms
func fullMaze() *Maze {
	z := emptyMaze()
	ySize := viper.GetInt("height")
	xSize := viper.GetInt("width")

	for y := 0; y < ySize; y++ {
		for x := 0; x < xSize; x++ {
			z.rooms[y][x].Walls = mazelib.Survey{true, true, true, true}
		}
	}

	return z
}

// TODO: Write your maze creator function here
func createMaze() *Maze {
	// TODO: Fill in the maze:
	// You need to insert a startingPoint for Icarus
	// You need to insert an EndingPoint (treasure) for Icarus
	// You need to Add and Remove walls as needed.
	// Use the mazelib.AddWall & mazelib.RmWall to do this
	rand.Seed(time.Now().UTC().UnixNano())
	var m *Maze
	r := rand.Intn(5)
	switch r {
	case 0, 1, 2:
		m = createBinaryTreeWithHoles()
	case 3:
		m = createBinaryTree()
	case 4:
		m = createGrowingTree()
	}
	//Insert Treasure
	xt := rand.Intn(viper.GetInt("width") - 1)
	yt := rand.Intn(viper.GetInt("height") - 1)
	m.SetTreasure(xt, yt)
	rand.Seed(time.Now().UTC().UnixNano())
	//Insert starting point

	xs := rand.Intn(viper.GetInt("width") - 1)
	ys := rand.Intn(viper.GetInt("height") - 1)
	//make sure, starting point is away from treasure
	for xs+ys == xt+yt {
		xs = rand.Intn(viper.GetInt("width") - 1)
		ys = rand.Intn(viper.GetInt("height") - 1)
	}
	m.SetStartPoint(xs, ys)

	return m

}

// based on the binary tree algorithm
func createBinaryTree() *Maze {
	// we can either make a connection to the room below or right from the current one
	m := fullMaze()
	for y := 0; y < m.Height(); y++ {
		for x := 0; x < m.Width(); x++ {

			dir := rand.Intn(2)
			// if we are at the right boarder, we can only go down
			if (y == m.Height()-1) && (x == m.Width()-1) {
				break
			} else if x == m.Width()-1 {
				dir = 1
			} else if y == m.Height()-1 {
				dir = 0
			}
			switch dir {
			case 0:
				m.rooms[y][x].RmWall(mazelib.E)
				m.rooms[y][x+1].RmWall(mazelib.W)
			case 1:
				m.rooms[y][x].RmWall(mazelib.S)
				m.rooms[y+1][x].RmWall(mazelib.N)
			}
		}
	}
	return m
}

// its based on the binary Tree algorithm, but sometimes, we add additional holes in the wall to create some loops
func createBinaryTreeWithHoles() *Maze {
	// we can either make a connection to the room below or right from the current one
	m := fullMaze()
	for y := 0; y < m.Height(); y++ {
		for x := 0; x < m.Width(); x++ {

			dir := rand.Intn(2)
			// if we are at the right boarder, we can only go down
			if (y == m.Height()-1) && (x == m.Width()-1) {
				break
			} else if x == m.Width()-1 {
				dir = 1
			} else if y == m.Height()-1 {
				dir = 0
			}
			switch dir {
			case 0:
				m.rooms[y][x].RmWall(mazelib.E)
				m.rooms[y][x+1].RmWall(mazelib.W)
			case 1:
				m.rooms[y][x].RmWall(mazelib.S)
				m.rooms[y+1][x].RmWall(mazelib.N)
			case 2:
				m.rooms[y][x].RmWall(mazelib.E)
				m.rooms[y][x+1].RmWall(mazelib.W)
				m.rooms[y][x].RmWall(mazelib.S)
				m.rooms[y+1][x].RmWall(mazelib.N)
			}
		}
	}
	return m
}

//growing tree algorithm
func createGrowingTree() *Maze {

	// starting with a full maze
	m := fullMaze()
	// create an 2D array for visited cells
	visited := make([][]bool, m.Width())
	for i := 0; i < m.Width(); i++ {
		visited[i] = make([]bool, m.Height())
	}
	// create an array for active cells
	cells := make([]mazelib.Coordinate, 1)
	//select a random starting point for the creation
	y := rand.Intn(m.Height() - 1)
	x := rand.Intn(m.Width() - 1)
	cells[0] = mazelib.Coordinate{x, y}
	visited[x][y] = true
	for len(cells) > 0 {
		//we use the newest cell to work with
		active := cells[len(cells)-1]
		//lets see if it has unvisited neighbors
		x, y = active.X, active.Y
		if (x == 0 || visited[x-1][y]) && (x == m.Width()-1 || visited[x+1][y]) && (y == 0 || visited[x][y-1]) && (y == m.Height()-1 || visited[x][y+1]) {
			cells = cells[:len(cells)-1]
		}
		//shuffle directions (up, down, left, right)
		dirs := rand.Perm(4)
		for _, d := range dirs {
			if !active.IsNil() {
				switch d {
				case 0: //up
					if (y > 0) && !(visited[x][y-1]) {
						//carve up, add that to cells, and visited, set active nil,
						active.X = -1
						cells = append(cells, mazelib.Coordinate{x, y - 1})
						m.rooms[y-1][x].RmWall(mazelib.S)
						m.rooms[y][x].RmWall(mazelib.N)
						visited[x][y-1] = true
					}
				case 1: //down
					if (y < m.Height()-1) && !(visited[x][y+1]) {
						//carve up, add that to cells, set active nil
						active.X = -1
						cells = append(cells, mazelib.Coordinate{x, y + 1})
						m.rooms[y][x].RmWall(mazelib.S)
						m.rooms[y+1][x].RmWall(mazelib.N)
						visited[x][y+1] = true
					}

				case 2: //left
					if (x > 0) && !(visited[x-1][y]) {
						//carve up, add that to cells, set active nil
						active.X = -1
						cells = append(cells, mazelib.Coordinate{x - 1, y})
						m.rooms[y][x].RmWall(mazelib.W)
						m.rooms[y][x-1].RmWall(mazelib.E)
						visited[x-1][y] = true
					}

				case 3: //right
					if (x < m.Width()-1) && !(visited[x+1][y]) {
						//carve up, add that to cells, set active nil
						active.X = -1
						cells = append(cells, mazelib.Coordinate{x + 1, y})
						m.rooms[y][x].RmWall(mazelib.E)
						m.rooms[y][x+1].RmWall(mazelib.W)
						visited[x+1][y] = true
					}
				}
			}
		}
	}

	return m
}
