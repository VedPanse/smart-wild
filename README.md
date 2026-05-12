# Orchestration Layer

This is an orchestration layer for wild-safe. It is responsible for communication across microservices and ensuring concurrency and parallelism along with incident report broadcasting across multiple clients.

Hosted at: https://smart-wild.onrender.com

## Incoming POST requests
The ml-service can make a http post request to the service via the following contract.

Strict contract:
```json
{
  "incident_id": "string",
  "type": "animal_on_road | person_on_road | stopped_vehicle | road_obstruction | unknown",
  "occurred_at": "ISO-8601 timestamp",
  "reported_at": "ISO-8601 timestamp",

  "location": {
    "latitude": "number",
    "longitude": "number",
    "road_name": "string | null",
    "direction": "string | null",
    "mile_marker": "string | null",
    "camera_id": "string"
  },

  "recommended_action": {
    "priority": "low | medium | high | critical",
    "message": "string"
  },

  "evidence": {
    "snapshot_url": "string | null",
    "video_clip_url": "string | null"
  }
}

```

Example:
```json
{
  "incident_id": "inc_20260509_194231_abc123",
  "type": "animal_on_road",
  "occurred_at": "2026-05-09T19:42:31.123Z",
  "reported_at": "2026-05-09T19:42:33.456Z",

  "location": {
    "latitude": 37.7749,
    "longitude": -122.4194,
    "road_name": "CA-1",
    "direction": "northbound",
    "mile_marker": "12.4",
    "camera_id": "rpi-roadside-001"
  },

  "recommended_action": {
    "priority": "high",
    "message": "Animal detected on or near roadway. Trigger roadside warning and notify nearby drivers."
  },

  "evidence": {
    "snapshot_url": "https://storage.example.com/incidents/inc_20260509_194231_abc123/frame.jpg",
    "video_clip_url": "https://storage.example.com/incidents/inc_20260509_194231_abc123/clip.mp4"
  }
}
```

## Remaining Tasks
- [ ] Complete the layer between flutter app and the Raspberry Pi so that a sound is played only when there are cars around.