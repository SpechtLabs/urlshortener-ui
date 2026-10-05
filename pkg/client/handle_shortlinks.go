package client

import (
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/attribute"

	"github.com/spechtlabs/urlshortener-ui/pkg/swagger"
)

// HandleHomePage lists the user's shortlinks.
func (c *UIClient) HandleHomePage(ct *gin.Context) {
	ctx, span := c.startSpan(ct, "UIClient.HandleHomePage")
	defer span.End()

	token, herr := loginToken(ct)
	if herr != nil {
		redirectToLogin(ct, herr)
		return
	}

	shortlinks, resp, err := c.apiClient.Apiv1Api.ApiV1ShortlinkGet(apiContext(ctx, token))
	if err != nil {
		span.RecordError(err)
		renderAPIError(ct, err, resp, "list your shortlinks")
		return
	}

	sort.Slice(shortlinks, func(i, j int) bool {
		return shortlinks[i].Name < shortlinks[j].Name
	})

	ct.HTML(http.StatusOK, "home.html", gin.H{
		"token":      token,
		"shortlinks": shortlinks,
		"copy_url":   c.config.ShortlinkURL,
	})
}

// HandleNew renders the form for a new shortlink.
func (c *UIClient) HandleNew(ct *gin.Context) {
	_, span := c.startSpan(ct, "UIClient.HandleNew")
	defer span.End()

	ct.HTML(http.StatusOK, "new.html", gin.H{})
}

// HandleNewShortlink creates the shortlink the new form describes, owned by
// the logged-in user.
func (c *UIClient) HandleNewShortlink(ct *gin.Context) {
	ctx, span := c.startSpan(ct, "UIClient.HandleNewShortlink")
	defer span.End()

	token, herr := loginToken(ct)
	if herr != nil {
		redirectToLogin(ct, herr)
		return
	}

	user, herr := c.getGhUser(ctx, token)
	if herr != nil {
		span.RecordError(herr)
		redirectToLogin(ct, herr)
		return
	}

	name, spec := shortlinkFromForm(ct)
	spec.Owner = user.Login
	span.SetAttributes(attribute.String("shortlink", name))

	if _, resp, err := c.apiClient.Apiv1Api.ApiV1ShortlinkShortlinkPost(apiContext(ctx, token), name, spec); writeFailed(resp, err) {
		span.RecordError(err)
		renderAPIError(ct, err, resp, "create the shortlink "+name)
		return
	}

	ct.Redirect(http.StatusFound, "/home")
}

// HandleEdit renders the form for the shortlink the name query parameter
// names.
func (c *UIClient) HandleEdit(ct *gin.Context) {
	ctx, span := c.startSpan(ct, "UIClient.HandleEdit")
	defer span.End()

	token, herr := loginToken(ct)
	if herr != nil {
		redirectToLogin(ct, herr)
		return
	}

	name := ct.Query("name")
	span.SetAttributes(attribute.String("shortlink", name))

	shortlink, resp, err := c.apiClient.Apiv1Api.ApiV1ShortlinkShortlinkGet(apiContext(ctx, token), name)
	if err != nil {
		span.RecordError(err)
		renderAPIError(ct, err, resp, "read the shortlink "+name)
		return
	}

	ct.HTML(http.StatusOK, "edit.html", gin.H{
		"edit_mode": true,
		"shortlink": shortlink,
		"coowners":  strings.Join(shortlink.Spec.Owners, ", "),
	})
}

// HandleEditShortlink writes the shortlink the edit form describes.
func (c *UIClient) HandleEditShortlink(ct *gin.Context) {
	ctx, span := c.startSpan(ct, "UIClient.HandleEditShortlink")
	defer span.End()

	token, herr := loginToken(ct)
	if herr != nil {
		redirectToLogin(ct, herr)
		return
	}

	name, spec := shortlinkFromForm(ct)
	spec.Owner = ct.PostForm("owner")
	span.SetAttributes(attribute.String("shortlink", name))

	if _, resp, err := c.apiClient.Apiv1Api.ApiV1ShortlinkShortlinkPut(apiContext(ctx, token), name, spec); writeFailed(resp, err) {
		span.RecordError(err)
		renderAPIError(ct, err, resp, "update the shortlink "+name)
		return
	}

	ct.Redirect(http.StatusFound, "/home")
}

// HandleDeleteShortlink deletes the shortlink the name query parameter names.
func (c *UIClient) HandleDeleteShortlink(ct *gin.Context) {
	ctx, span := c.startSpan(ct, "UIClient.HandleDeleteShortlink")
	defer span.End()

	token, herr := loginToken(ct)
	if herr != nil {
		redirectToLogin(ct, herr)
		return
	}

	name := ct.Query("name")
	span.SetAttributes(attribute.String("shortlink", name))

	if _, resp, err := c.apiClient.Apiv1Api.ApiV1ShortlinkShortlinkDelete(apiContext(ctx, token), name); writeFailed(resp, err) {
		span.RecordError(err)
		renderAPIError(ct, err, resp, "delete the shortlink "+name)
		return
	}

	ct.Redirect(http.StatusFound, "/home")
}

// HandleNotFound renders the 404 page.
func (c *UIClient) HandleNotFound(ct *gin.Context) {
	_, span := c.startSpan(ct, "UIClient.HandleNotFound")
	defer span.End()

	span.AddEvent("Not found")

	ct.HTML(http.StatusNotFound, "404.html", gin.H{})
}

// shortlinkFromForm reads the shortlink's name and spec from the new or edit
// form. A redirect through the HTML page is code 200; fields that don't
// parse as numbers are 0.
func shortlinkFromForm(ct *gin.Context) (string, swagger.V1alpha1ShortLinkSpec) {
	redirectAfter, _ := strconv.ParseInt(ct.PostForm("redirectAfter"), 10, 32)
	code, _ := strconv.ParseInt(ct.PostForm("httpStatusCode"), 10, 32)

	if ct.PostForm("redirectTypeOption") == "html" {
		code = http.StatusOK
	}

	var coOwners []string
	for owner := range strings.SplitSeq(ct.PostForm("co-owners"), ",") {
		if owner = strings.TrimSpace(owner); owner != "" {
			coOwners = append(coOwners, owner)
		}
	}

	return ct.PostForm("name"), swagger.V1alpha1ShortLinkSpec{
		After:  int32(redirectAfter),
		Code:   int32(code),
		Owners: coOwners,
		Target: ct.PostForm("url"),
	}
}
