package com.sharedisk.lan;

import org.json.JSONObject;

final class TransferInfo {
    final String id;
    final String objectId;
    final String targetDeviceId;
    final String state;
    final int attempt;

    TransferInfo(JSONObject json) {
        id = json.optString("id");
        objectId = json.optString("object_id");
        targetDeviceId = json.optString("target_device_id");
        state = json.optString("state");
        attempt = json.optInt("attempt");
    }

    boolean terminal() {
        return state.equals("completed") || state.equals("canceled") || state.equals("failed_permanent");
    }
}
