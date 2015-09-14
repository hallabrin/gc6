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
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"time"

	"bitbucket.org/mannih/gc6/mazelib"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Defining the icarus command.
// This will be called as 'laybrinth icarus'
var icarusCmd = &cobra.Command{
	Use:     "icarus",
	Aliases: []string{"client"},
	Short:   "Start the laybrinth solver",
	Long: `Icarus wakes up to find himself in the middle of a Labyrinth.
  Due to the darkness of the Labyrinth he can only see his immediate cell and if
  there is a wall or not to the top, right, bottom and left. He takes one step
  and then can discover if his new cell has walls on each of the four sides.

  Icarus can connect to a Daedalus and solve many laybrinths at a time.`,
	Run: func(cmd *cobra.Command, args []string) {
		RunIcarus()
	},
}

var opposite = map[string]string{"up": "down", "down": "up", "left": "right", "right": "left"}

func init() {
	RootCmd.AddCommand(icarusCmd)
}

func RunIcarus() {
	// Run the solver as many times as the user desires.
	fmt.Println("Solving", viper.GetInt("times"), "times")
	for x := 0; x < viper.GetInt("times"); x++ {

		solveMaze()
	}

	// Once we have solved the maze the required times, tell daedalus we are done
	makeRequest("http://127.0.0.1:" + viper.GetString("port") + "/done")
}

// Make a call to the laybrinth server (daedalus) that icarus is ready to wake up
func awake() mazelib.Survey {
	contents, err := makeRequest("http://127.0.0.1:" + viper.GetString("port") + "/awake")
	if err != nil {
		fmt.Println(err)
	}
	r := ToReply(contents)
	return r.Survey
}

// Make a call to the laybrinth server (daedalus)
// to move Icarus a given direction
// Will be used heavily by solveMaze
func Move(direction string) (mazelib.Survey, error) {
	if direction == "left" || direction == "right" || direction == "up" || direction == "down" {

		contents, err := makeRequest("http://127.0.0.1:" + viper.GetString("port") + "/move/" + direction)
		if err != nil {
			return mazelib.Survey{}, err
		}

		rep := ToReply(contents)
		if rep.Victory == true {
			fmt.Println(rep.Message)
			// os.Exit(1)
			return rep.Survey, mazelib.ErrVictory
		} else {
			if rep.Message == "" {
				return rep.Survey, nil
			} else {

				return rep.Survey, errors.New(rep.Message)
			}
		}
	}

	return mazelib.Survey{}, errors.New("invalid direction")
}

// utility function to wrap making requests to the daedalus server
func makeRequest(url string) ([]byte, error) {
	response, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	contents, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}
	return contents, nil
}

// Handling a JSON response and unmarshalling it into a reply struct
func ToReply(in []byte) mazelib.Reply {
	res := &mazelib.Reply{}
	json.Unmarshal(in, &res)
	return *res
}

// TODO: This is where you work your magic
func solveMaze() {
	s := awake() // Need to start with waking up to initialize a new maze
	// You'll probably want to set this to a named value and start by figuring
	// out which step to take next
	//TODO: Write your solver algorithm here
	nextMove(s, "")
}

// Recursive function. s is the result of the move function, dir the direction we just moved
func nextMove(s mazelib.Survey, dir string) bool {
	// try to move in one direction, unless there is a wall
	// unless it returns the victory error, we call this function recurisvely
	// returns true if victory
	// returns false if a dead end (only possible direction is the one we came from)
	var possibilities []string

	if !s.Bottom && !(dir == "up") {
		possibilities = append(possibilities, "down")
	}
	if !s.Left && !(dir == "right") {
		possibilities = append(possibilities, "left")
	}
	if !s.Right && !(dir == "left") {
		possibilities = append(possibilities, "right")
	}
	if !s.Top && !(dir == "down") {
		possibilities = append(possibilities, "up")
	}
	if len(possibilities) == 0 {
		return false
	}
	// if there are more then one possible direction, lets shuffle.

	if len(possibilities) > 1 {
		possibilities = shuffle(possibilities)
	}

	//now try all possibilities
	for _, d := range possibilities {
		result, err := Move(d)
		if err != nil {
			if err == mazelib.ErrVictory {
				return true
			} else {
				fmt.Println(err.Error())
			}
		}
		if nextMove(result, d) == false {
			// the move was negative, so lets go back one step
			if _, err := Move(opposite[d]); err != nil {
				fmt.Println(err.Error())
			}

		} else {

			return true
		}

	}
	return false
}

func shuffle(p []string) []string {
	rand.Seed(time.Now().UnixNano())
	temp := make([]string, len(p))
	t := rand.Perm(len(p))
	for i, j := range t {
		temp[i] = p[j]
	}
	return temp
}
