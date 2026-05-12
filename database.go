package main

import (
	"context"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5"
)

func sendToDatabase(incident Incident) {
	var connString = os.Getenv("EXTERNAL_DATABASE_URL")
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, connString)

	if nil != err {
		fmt.Printf("Some error with database: %s", err)
		return
	}

	defer conn.Close(ctx)
	fmt.Println("Connected")

	tx, err := conn.Begin(ctx)
	if err != nil {
		fmt.Printf("failed to begin database transaction: %v\n", err)
		return
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(
		ctx,
		`
		INSERT INTO cameras (
			id,
			road_name,
			direction,
			mile_marker,
			latitude,
			longitude
		)
		VALUES (
			$1,$2,$3,$4,$5,$6
		)
		ON CONFLICT (id) DO UPDATE SET
			road_name = EXCLUDED.road_name,
			direction = EXCLUDED.direction,
			mile_marker = EXCLUDED.mile_marker,
			latitude = EXCLUDED.latitude,
			longitude = EXCLUDED.longitude
		`,
		incident.Location.CameraID,
		incident.Location.RoadName,
		incident.Location.Direction,
		incident.Location.MileMarker,
		incident.Location.Latitude,
		incident.Location.Longitude,
	)
	if err != nil {
		fmt.Printf("failed to upsert camera: %v\n", err)
		return
	}

	_, err = tx.Exec(
		ctx,
		`
			INSERT INTO incidents (
				id,
			type,
			occurred_at,
			reported_at,
			latitude,
			longitude,
			road_name,
			direction,
			mile_marker,
			camera_id,
			priority,
			recommended_message,
			snapshot_url,
			video_clip_url
		)
		VALUES (
			$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14
		)
		`,
		incident.ID,
		incident.Type,
		incident.OccurredAt,
		incident.ReportedAt,
		incident.Location.Latitude,
		incident.Location.Longitude,
		incident.Location.RoadName,
		incident.Location.Direction,
		incident.Location.MileMarker,
		incident.Location.CameraID,
		incident.RecommendedAction.Priority,
		incident.RecommendedAction.Message,
		incident.Evidence.SnapshotURL,
		incident.Evidence.VideoClipURL,
	)
	if err != nil {
		fmt.Printf("failed to insert incident: %v\n", err)
		return
	}

	if err := tx.Commit(ctx); err != nil {
		fmt.Printf("failed to commit incident insert: %v\n", err)
	} else {
		fmt.Println("Added row to database")
	}

}
