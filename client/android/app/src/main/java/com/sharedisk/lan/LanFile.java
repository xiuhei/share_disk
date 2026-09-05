package com.sharedisk.lan;

import org.json.JSONObject;

final class LanFile {
    final String id;
	final String localFileId;
    final String objectId;
    final String name;
    final String mime;
    final long size;
    final String sha256;
    final String status;
    final String deletedAt;
    final String purgeAfter;
	final String replicaEndpoint;
	final String originEndpoint;
	final String contentLocalFileId;
	final String originDeviceName;
	final String originDeviceId;
	final String originLastSeenAt;

    LanFile(JSONObject json) {
        id = json.optString("id");
		localFileId = json.optString("local_file_id", id);
        objectId = json.optString("object_id");
        name = json.optString("name");
        mime = json.optString("mime", "application/octet-stream");
        size = json.optLong("size");
        sha256 = json.optString("sha256");
        status = json.optString("status", "active");
        deletedAt = json.optString("deleted_at");
        purgeAfter = json.optString("purge_after");
		replicaEndpoint = json.optString("replica_endpoint");
		originEndpoint = json.optString("origin_endpoint", replicaEndpoint);
		contentLocalFileId = json.optString("content_local_file_id", localFileId);
		originDeviceName = json.optString("origin_device_name");
		originDeviceId = json.optString("origin_device_id");
		originLastSeenAt = json.optString("origin_last_seen_at");
    }
}
