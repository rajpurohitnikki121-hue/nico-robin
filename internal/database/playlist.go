/*
 * ● YukkiMusic
 * ○ A high-performance engine for streaming music in Telegram voicechats.
 *
 * Copyright (C) 2026 TheTeamVivek
 *
 * This program is free software: you can redistribute it and/or modify it under the
 * terms of the GNU General Public License as published by the Free Software Foundation,
 * either version 3 of the License, or (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful, but WITHOUT ANY
 * WARRANTY; without even the implied warranty of MERCHANTABILITY or FITNESS FOR A
 * PARTICULAR PURPOSE. See the GNU General Public License for more details.
 *
 * Repository: https://github.com/TheTeamVivek/YukkiMusic
 */

// NOTE: This is a NEW file. It does not modify database.go or any other
// existing database file. It lazily obtains its own MongoDB collections
// from the already-initialized `database` package variable, so no changes
// to Init() are required for this feature to work.

package database

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// Limits as required by the feature spec.
const (
	MaxPlaylistsPerUser = 5
	MaxSongsPerPlaylist = 50
)

var (
	playlistColl      *mongo.Collection
	playlistStateColl *mongo.Collection
	playlistCollOnce  sync.Once
)

var (
	ErrPlaylistLimit     = errors.New("playlist limit reached")
	ErrPlaylistExists    = errors.New("playlist already exists")
	ErrPlaylistNotFound  = errors.New("playlist not found")
	ErrSongLimit         = errors.New("song limit reached")
	ErrSongNotFound      = errors.New("song not found in playlist")
	ErrNoCurrentPlaylist = errors.New("no current playlist selected")
)

// ensurePlaylistColls lazily binds the playlist collections once the
// package-level `database` handle (set by database.Init) is ready.
func ensurePlaylistColls() {
	playlistCollOnce.Do(func() {
		playlistColl = database.Collection("playlists")
		playlistStateColl = database.Collection("playlist_state")
	})
}

// PlaylistSong stores only the minimum metadata required to replay a song
// later. Artwork/thumbnail is intentionally NEVER stored here to keep
// database storage lightweight.
type PlaylistSong struct {
	ID       string `bson:"id"`
	Title    string `bson:"title"`
	URL      string `bson:"url"`
	Duration int    `bson:"duration"`
	Source   string `bson:"source"`
}

// Playlist represents a single user-owned playlist.
type Playlist struct {
	UserID    int64          `bson:"user_id"`
	Name      string         `bson:"name"`
	Songs     []PlaylistSong `bson:"songs"`
	CreatedAt time.Time      `bson:"created_at"`
}

type playlistStateDoc struct {
	UserID  int64  `bson:"_id"`
	Current string `bson:"current"`
}

// CreatePlaylist creates a new playlist for the user and makes it the
// user's current playlist. Enforces MaxPlaylistsPerUser.
func CreatePlaylist(userID int64, name string) error {
	ensurePlaylistColls()
	c, cancel := ctx()
	defer cancel()

	count, err := playlistColl.CountDocuments(c, bson.M{"user_id": userID})
	if err != nil {
		return err
	}
	if count >= MaxPlaylistsPerUser {
		return ErrPlaylistLimit
	}

	existing, err := playlistColl.CountDocuments(c, bson.M{"user_id": userID, "name": name})
	if err != nil {
		return err
	}
	if existing > 0 {
		return ErrPlaylistExists
	}

	pl := Playlist{
		UserID:    userID,
		Name:      name,
		Songs:     []PlaylistSong{},
		CreatedAt: time.Now(),
	}
	if _, err := playlistColl.InsertOne(c, pl); err != nil {
		return err
	}

	return setCurrentPlaylist(c, userID, name)
}

// AddSongToPlaylist appends a song to the user's current playlist.
// Enforces MaxSongsPerPlaylist.
func AddSongToPlaylist(userID int64, song PlaylistSong) error {
	ensurePlaylistColls()
	c, cancel := ctx()
	defer cancel()

	name, err := currentPlaylistName(c, userID)
	if err != nil {
		return err
	}

	var pl Playlist
	if err := playlistColl.FindOne(c, bson.M{"user_id": userID, "name": name}).Decode(&pl); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return ErrPlaylistNotFound
		}
		return err
	}
	if len(pl.Songs) >= MaxSongsPerPlaylist {
		return ErrSongLimit
	}

	_, err = playlistColl.UpdateOne(
		c,
		bson.M{"user_id": userID, "name": name},
		bson.M{"$push": bson.M{"songs": song}},
	)
	return err
}

// RemoveSongFromPlaylist removes the first song matching the given title
// (case-insensitive) from the user's current playlist. This ONLY edits the
// saved playlist — it never touches an active playback queue.
func RemoveSongFromPlaylist(userID int64, title string) error {
	ensurePlaylistColls()
	c, cancel := ctx()
	defer cancel()

	name, err := currentPlaylistName(c, userID)
	if err != nil {
		return err
	}

	var pl Playlist
	if err := playlistColl.FindOne(c, bson.M{"user_id": userID, "name": name}).Decode(&pl); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return ErrPlaylistNotFound
		}
		return err
	}

	idx := -1
	for i, s := range pl.Songs {
		if strings.EqualFold(s.Title, title) {
			idx = i
			break
		}
	}
	if idx == -1 {
		return ErrSongNotFound
	}

	newSongs := append(pl.Songs[:idx], pl.Songs[idx+1:]...)

	_, err = playlistColl.UpdateOne(
		c,
		bson.M{"user_id": userID, "name": name},
		bson.M{"$set": bson.M{"songs": newSongs}},
	)
	return err
}

// UserPlaylists returns all playlists belonging to the user.
func UserPlaylists(userID int64) ([]Playlist, error) {
	ensurePlaylistColls()
	c, cancel := ctx()
	defer cancel()

	cur, err := playlistColl.Find(c, bson.M{"user_id": userID})
	if err != nil {
		return nil, err
	}
	defer cur.Close(c)

	var playlists []Playlist
	if err := cur.All(c, &playlists); err != nil {
		return nil, err
	}
	return playlists, nil
}

// GetPlaylist fetches a specific playlist by name for the user.
func GetPlaylist(userID int64, name string) (*Playlist, error) {
	ensurePlaylistColls()
	c, cancel := ctx()
	defer cancel()

	var pl Playlist
	err := playlistColl.FindOne(c, bson.M{"user_id": userID, "name": name}).Decode(&pl)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrPlaylistNotFound
		}
		return nil, err
	}
	return &pl, nil
}

// CurrentPlaylist returns the name of the user's currently selected playlist.
func CurrentPlaylist(userID int64) (string, error) {
	ensurePlaylistColls()
	c, cancel := ctx()
	defer cancel()
	return currentPlaylistName(c, userID)
}

// SetCurrentPlaylist sets the user's current playlist by name.
func SetCurrentPlaylist(userID int64, name string) error {
	ensurePlaylistColls()
	c, cancel := ctx()
	defer cancel()
	return setCurrentPlaylist(c, userID, name)
}

func setCurrentPlaylist(c context.Context, userID int64, name string) error {
	_, err := playlistStateColl.UpdateOne(
		c,
		bson.M{"_id": userID},
		bson.M{"$set": bson.M{"current": name}},
		upsertOpt,
	)
	return err
}

func currentPlaylistName(c context.Context, userID int64) (string, error) {
	var doc playlistStateDoc
	err := playlistStateColl.FindOne(c, bson.M{"_id": userID}).Decode(&doc)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return "", ErrNoCurrentPlaylist
		}
		return "", err
	}
	if doc.Current == "" {
		return "", ErrNoCurrentPlaylist
	}
	return doc.Current, nil
}
