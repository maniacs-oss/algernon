package main

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"github.com/xyproto/permissions2"
	"github.com/xyproto/term"
	"github.com/yuin/gopher-lua"
	"strings"
)

const (
	helpText = `Available functions:

Data structures

// Get or create Redis-backed Set (takes a name, returns a set object)
Set(string) -> userdata
// Add an element to the set
set:add(string)
// Remove an element from the set
set:del(string)
// Check if a set contains a value. Returns true only if the value exists and there were no errors.
set:has(string) -> bool
// Get all members of the set
set:getall() -> table
// Remove the set itself. Returns true if successful.
set:remove() -> bool

// Get or create a Redis-backed List (takes a name, returns a list object)
List(string) -> userdata
// Add an element to the list
list:add(string)
// Get all members of the list
list:getall() -> table
// Get the last element of the list. The returned value can be empty
list:getlast() -> string
// Get the N last elements of the list
list:getlastn(number) -> table
// Remove the list itself. Returns true if successful.
list:remove() -> bool

// Get or create a Redis-backed HashMap (takes a name, returns a hash map object)
HashMap(string) -> userdata
// For a given element id (for instance a user id), set a key. Returns true if successful.
hash:set(string, string, string) -> bool
// For a given element id (for instance a user id), and a key, return a value.
hash:get(string, string) -> string
// For a given element id (for instance a user id), and a key, check if the key exists in the hash map.
hash:has(string, string) -> bool
// For a given element id (for instance a user id), check if it exists.
hash:exists(string) -> bool
// Get all keys of the hash map
hash:getall() -> table
// Remove a key for an entry in a hash map. Returns true if successful
hash:delkey(string, string) -> bool
// Remove an element (for instance a user). Returns true if successful
hash:del(string) -> bool
// Remove the hash map itself. Returns true if successful.
hash:remove() -> bool

// Get or create a Redis-backed KeyValue collection (takes a name, returns a key/value object)
KeyValue(string) -> userdata
// Set a key and value. Returns true if successful.
kv:set(string, string) -> bool
// Takes a key, returns a value. Returns an empty string if the function fails.
kv:get(string) -> string
// Takes a key, returns the value+1. Creates a key/value and returns "1" if it did not already exist.
kv:inc(string) -> string
// Remove a key. Returns true if successful.
kv:del(string) -> bool
// Remove the KeyValue itself. Returns true if successful.
kv:remove() -> bool

Server configuration

// Set the default address for the server on the form [host][:port].
SetAddr(string)
// Reset the URL prefixes and make everything *public*.
ClearPermissions()
// Add an URL prefix that will have *admin* rights.
AddAdminPrefix(string)
// Add an URL prefix that will have *user* rights.
AddUserPrefix(string)
// Provide a lua function that will be used as the permission denied handler.
DenyHandler(function)
// Log to the given filename. If the filename is an empty string, log to stderr. Returns true if successful.
LogTo(string) -> bool
// Provide a lua function that will be run once, when the server is ready to start serving.
OnReady(function)

Output

// Log the given strings as INFO. Takes a variable number of strings.
log(...)
// Log the given strings as WARN. Takes a variable number of strings.
warn(...)
// Log the given strings as an error. Takes a variable number of strings.
error(...)
// Output text. Takes a variable number of strings.
print(...)

Various

// Return a string with various server information
ServerInfo() -> string
// Return the version string for the server
version() -> string
// Marshall a table to JSON
JSON(table) -> string
// Try to extract the contents of a Lua value
pprint(value) -> string
// Sleep the given number of seconds (can be a float)
sleep(number)
`
)

// Attempt to return a more informative text than the memory location
func pprint(value lua.LValue) string {
	switch v := value.(type) {
	case *lua.LTable:
		mapinterface, multiple := table2map(v)
		if multiple {
			return v.String()
		}
		switch m := mapinterface.(type) {
		case map[string]string:
			return fmt.Sprintf("%v", map[string]string(m))
		case map[string]int:
			return fmt.Sprintf("%v", map[string]int(m))
		case map[int]string:
			return fmt.Sprintf("%v", map[int]string(m))
		case map[int]int:
			return fmt.Sprintf("%v", map[int]int(m))
		default:
			return v.String()
		}
	case *lua.LFunction:
		if v.Proto != nil {
			// Extended information about the function
			return v.Proto.String()
		}
		return v.String()
	default:
		return v.String()
	}
}

// Export Lua functions related to the REPL
func exportREPL(L *lua.LState) {

	// Attempt to return a more informative text than the memory location
	L.SetGlobal("pprint", L.NewFunction(func(L *lua.LState) int {
		L.Push(lua.LString(pprint(L.Get(1))))
		return 1 // number of results
	}))

}

// Split the given line in two parts, and color the parts
func colorSplit(line, sep string, colorFunc1, colorFuncSep, colorFunc2 func(string) string, reverse bool) (string, string) {
	if strings.Contains(line, sep) {
		fields := strings.SplitN(line, sep, 2)
		s1 := ""
		if colorFunc1 != nil {
			s1 += colorFunc1(fields[0])
		} else {
			s1 += fields[0]
		}
		s2 := ""
		if colorFunc2 != nil {
			s2 += colorFuncSep(sep) + colorFunc2(fields[1])
		} else {
			s2 += sep + fields[1]
		}
		return s1, s2
	}
	if reverse {
		return "", line
	}
	return line, ""
}

// Syntax highlight the given line
func highlight(o *term.TextOutput, line string) string {
	unprocessed := line
	unprocessed, comment := colorSplit(unprocessed, "//", nil, o.DarkGray, o.DarkGray, false)
	module, unprocessed := colorSplit(unprocessed, ":", o.LightGreen, o.DarkRed, nil, true)
	function := ""
	if unprocessed != "" {
		// Green function names
		if strings.Contains(unprocessed, "(") {
			fields := strings.SplitN(unprocessed, "(", 2)
			function = o.LightGreen(fields[0])
			unprocessed = "(" + fields[1]
		}
	}
	unprocessed, typed := colorSplit(unprocessed, "->", nil, o.LightBlue, o.DarkRed, false)
	unprocessed = strings.Replace(unprocessed, "string", o.LightBlue("string"), -1)
	unprocessed = strings.Replace(unprocessed, "number", o.LightYellow("number"), -1)
	unprocessed = strings.Replace(unprocessed, "function", o.LightCyan("function"), -1)
	return module + function + unprocessed + typed + comment
}

// The REPL
// TODO: Use readline
func REPL(perm *permissions.Permissions, luapool *lStatePool) error {

	// Retrieve the userstate
	userstate := perm.UserState()

	// Retrieve a Lua state
	L := luapool.Get()
	// Don't re-use the Lua state
	defer L.Close()

	// Server configuration functions
	exportServerConfigFunctions(L, perm, "", luapool)

	// Other basic system functions, like log()
	exportBasicSystemFunctions(L)

	// Simpleredis data structures
	exportList(L, userstate)
	exportSet(L, userstate)
	exportHash(L, userstate)
	exportKeyValue(L, userstate)

	// For handling JSON data
	exportJSONFunctions(L)

	// Pretty printing
	exportREPL(L)

	// Colors and input
	o := term.NewTextOutput(true, true)

	o.Println(o.LightGreen(versionString))
	o.Println(o.LightGreen("Ready"))

	var (
		line        string
		err         error
		printWorked bool
	)
	for {
		// Retrieve user input
		line = strings.TrimSpace(term.Ask(o.LightGreen("lua> ")))

		switch line {
		case "help":
			for _, line := range strings.Split(helpText, "\n") {
				o.Println(highlight(o, line))
			}
			continue
		case "zalgo":
			// Easter egg
			o.ErrExit("Ḫ̷̲̫̰̯̭̀̂̑̈ͅĚ̥̖̩̘̱͔͈͈ͬ̚ ̦̦͖̲̀ͦ͂C̜͓̲̹͐̔ͭ̏Oͭ͛͂̋ͭͬͬ͆͏̺͓̰͚͠ͅM̢͉̼̖͍̊̕Ḛ̭̭͗̉̀̆ͬ̐ͪ̒S͉̪͂͌̄")
			return nil
		}

		// If the line doesn't start with print, try adding it
		printWorked = false
		if !strings.HasPrefix(line, "print(") {
			printWorked = nil == L.DoString("print(pprint("+line+"))")
		}
		if !printWorked {
			if err = L.DoString(line); err != nil {
				// Output the original error message
				log.Error(err)
			}
		}
	}
}