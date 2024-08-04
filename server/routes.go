package server

import (
	"github.com/gin-gonic/gin"
	"github.com/rmorlok/authproxy/models"
	"net/http"
)

// album represents data about a record album.
type domain struct {
	Id          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// albums slice to seed record album data.
var allDomains = []models.Domain{
	{Id: "notifications", Name: "Notifications", Description: "Integrations for notification methods within the application"},
	{Id: "integrations", Name: "Integrations", Description: "Integrations offered for the purposed of integrating the with the Acme product"},
	{Id: "other_app", Name: "Other App Integrations", Description: "Integrations for a completely separate app we offer"},
}

func domainJsonfromModel(m models.Domain) domain {
	return domain{
		Id:          m.Id,
		Name:        m.Name,
		Description: m.Description,
	}
}

// ListDomains responds with the list of all domains as JSON
func ListDomains(c *gin.Context) {
	domains := make([]domain, 0, len(allDomains))
	for _, d := range allDomains {
		domains = append(domains, domainJsonfromModel(d))
	}

	c.IndentedJSON(http.StatusOK, domains)
}

// PostAlbums adds an album from JSON received in the request body.
//func PostAlbums(c *gin.Context) {
//	var newAlbum album
//
//	// Call BindJSON to bind the received JSON to
//	// newAlbum.
//	if err := c.BindJSON(&newAlbum); err != nil {
//		return
//	}
//
//	// Add the new album to the slice.
//	albums = append(albums, newAlbum)
//	c.IndentedJSON(http.StatusCreated, newAlbum)
//}

// GetAlbumByID locates the album whose ID value matches the id
// parameter sent by the client, then returns that album as a response.
//func GetAlbumByID(c *gin.Context) {
//	id := c.Param("id")
//
//	// Loop through the list of albums, looking for
//	// an album whose ID value matches the parameter.
//	for _, a := range albums {
//		if a.ID == id {
//			c.IndentedJSON(http.StatusOK, a)
//			return
//		}
//	}
//	c.IndentedJSON(http.StatusNotFound, gin.H{"message": "album not found"})
//}
