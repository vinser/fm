// Simple filemailer.
// Useful for sending emails with attached files like multivolume archives.
package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"net/smtp"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/jordan-wright/email"
	"github.com/spf13/viper"
)

// init is a special Go function that is automatically called before the program starts.
//
// This function initializes the configuration settings for the program by parsing command line
// flags and reading a configuration file. It expects a flag named "config" to specify the name
// of the configuration file (without the toml extension). The configuration file is searched
// for in the current directory and in the "/etc/fm" directory.
//
// The function returns no value.
func init() {
	var config string
	flag.StringVar(&config, "config", "fm", "The name of the config file. Must not include extension.")
	flag.Parse()
	viper.SetConfigName(config)
	viper.AddConfigPath(".")
	viper.AddConfigPath("/etc/fm")
	if err := viper.ReadInConfig(); err != nil {
		log.Fatalf("Fatal error reading config file: %v", err)
	}
}

// main function creates a new watcher using fsnotify.NewWatcher() and starts listening for events.
//
// The watch folder and extensions are retrieved from the viper configuration.
// It then loops through the events received from the watcher and handles them accordingly.
// If the event is a create event and the file extension matches the allowed extensions,
// the file is emailed to the addressees and moved to the save folder.
// If the event is an error event, it is logged.
// Finally, the watch folder is added to the watcher and the program waits for user input to stop watching.
func main() {
	// Create new watcher.
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal("Fatal error creating watcher:", err)
	}
	defer watcher.Close()

	// Start listening for events.
	watch := viper.GetString("watch.folder")
	filetypes := viper.GetStringSlice("watch.filetypes")
	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if event.Has(fsnotify.Create) {
					base := filepath.Base(event.Name)
					if IsFile(event.Name) {
						if ExtensionMatched(filetypes, filepath.Ext(event.Name)) {
							err := EmailFile(event.Name)
							if err != nil {
								log.Println(err)
								break
							}
							log.Println("File:", base, "has been sent to addressees")
							os.MkdirAll(filepath.Join(watch, "save"), 0750)
							os.Rename(event.Name, filepath.Join(watch, "save", base))
						} else {
							log.Println("File:", base, "has been ignored by extension")
						}
					}
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Println("Watcher error:", err)
			}
		}
	}()

	err = watcher.Add(watch)
	if err != nil {
		log.Fatal(err)
	}
	log.Println("Watching folder:", watch)
	log.Println("Addressees:", viper.GetStringSlice("email.addressees"))
	log.Println("Press Enter to stop watching.")
	fmt.Scanln()
}

// EmailFile sends an email with an attached file.
//
// The function takes a file path as a parameter and returns an error if there is any.
func EmailFile(file string) error {
	time.Sleep(time.Second)
	base := filepath.Base(file)

	em := email.NewEmail()
	em.From = viper.GetString("email.sender")
	em.To = viper.GetStringSlice("email.addressees")
	em.Subject = base
	em.Text = []byte(base)

	if _, err := em.AttachFile(file); err != nil {
		return fmt.Errorf("attach file %s\n error: %s", base, err.Error())
	}

	host := viper.GetString("smtp.host")
	port := viper.GetString("smtp.port")
	username := viper.GetString("smtp.username")
	password := viper.GetString("smtp.password")
	t := &tls.Config{InsecureSkipVerify: true, ServerName: host}
	auth := smtp.PlainAuth("", username, password, host)

	if err := em.SendWithTLS(host+":"+port, auth, t); err != nil {
		return fmt.Errorf("send file %s\n error: %s", base, err.Error())
	}

	return nil
}

// ExtensionMatched checks if the given extension matches any of the templates.
//
// Parameters:
// - templates: a slice of strings representing the templates to match against.
// - ext: a string representing the extension to be checked.
//
// Return type:
// - bool: a boolean value indicating whether the extension matches any of the templates.
func ExtensionMatched(templates []string, ext string) bool {
	x := strings.TrimPrefix(ext, ".")
	for _, pattern := range templates {
		if matched, _ := regexp.MatchString(pattern, x); matched {
			return true
		}
	}
	return false
}

// IsFile checks if the given path is a file.
//
// path: the path to the file.
// returns: a boolean indicating if the path is a file or not.
func IsFile(path string) bool {
	fileInfo, err := os.Stat(path)
	return err == nil && !fileInfo.IsDir()
}
