package com.sharedisk.lan;

import android.content.Context;
import android.content.SharedPreferences;
import android.security.keystore.KeyGenParameterSpec;
import android.security.keystore.KeyProperties;

import java.nio.charset.StandardCharsets;
import java.security.KeyStore;
import java.util.Base64;

import javax.crypto.Cipher;
import javax.crypto.KeyGenerator;
import javax.crypto.SecretKey;
import javax.crypto.spec.GCMParameterSpec;

/** Stores session credentials encrypted by a non-exportable Android Keystore key. */
final class SecureCredentials {
    private static final String KEYSTORE = "AndroidKeyStore";
    private static final String KEY_ALIAS = "share_disk_session_v1";
    private static final String PREFIX = "encrypted_";
    private static final String TRANSFORMATION = "AES/GCM/NoPadding";

    private final SharedPreferences prefs;

    SecureCredentials(Context context, SharedPreferences prefs) {
        this.prefs = prefs;
    }

    String get(String name) throws Exception {
        String encoded = prefs.getString(PREFIX + name, "");
        if (!encoded.isEmpty()) return decrypt(name, encoded);

        // One-time migration for APKs that stored tokens in plain preferences.
        String legacy = prefs.getString(name, "");
        if (!legacy.isEmpty()) {
            put(name, legacy);
            prefs.edit().remove(name).apply();
        }
        return legacy;
    }

    void put(String name, String value) throws Exception {
        Cipher cipher = Cipher.getInstance(TRANSFORMATION);
        cipher.init(Cipher.ENCRYPT_MODE, key());
        cipher.updateAAD(name.getBytes(StandardCharsets.UTF_8));
        byte[] ciphertext = cipher.doFinal(value.getBytes(StandardCharsets.UTF_8));
        byte[] packed = new byte[1 + cipher.getIV().length + ciphertext.length];
        packed[0] = (byte) cipher.getIV().length;
        System.arraycopy(cipher.getIV(), 0, packed, 1, cipher.getIV().length);
        System.arraycopy(ciphertext, 0, packed, 1 + cipher.getIV().length, ciphertext.length);
        if (!prefs.edit().putString(PREFIX + name, Base64.getEncoder().encodeToString(packed)).commit()) {
            throw new IllegalStateException("无法持久保存登录凭据");
        }
        prefs.edit().remove(name).apply();
    }

    void clearSession() {
        prefs.edit()
                .remove(PREFIX + "access_token")
                .remove(PREFIX + "refresh_token")
                .remove("access_token")
                .remove("refresh_token")
                .remove("access_expires_at")
                .apply();
    }

    private String decrypt(String name, String encoded) throws Exception {
        byte[] packed = Base64.getDecoder().decode(encoded);
        if (packed.length < 14) throw new IllegalStateException("登录凭据已损坏");
        int ivLength = packed[0] & 0xff;
        if (ivLength < 12 || 1 + ivLength >= packed.length) throw new IllegalStateException("登录凭据已损坏");
        byte[] iv = new byte[ivLength];
        System.arraycopy(packed, 1, iv, 0, ivLength);
        Cipher cipher = Cipher.getInstance(TRANSFORMATION);
        cipher.init(Cipher.DECRYPT_MODE, key(), new GCMParameterSpec(128, iv));
        cipher.updateAAD(name.getBytes(StandardCharsets.UTF_8));
        return new String(cipher.doFinal(packed, 1 + ivLength, packed.length - 1 - ivLength), StandardCharsets.UTF_8);
    }

    private SecretKey key() throws Exception {
        KeyStore store = KeyStore.getInstance(KEYSTORE);
        store.load(null);
        KeyStore.Entry existing = store.getEntry(KEY_ALIAS, null);
        if (existing instanceof KeyStore.SecretKeyEntry) {
            return ((KeyStore.SecretKeyEntry) existing).getSecretKey();
        }
        KeyGenerator generator = KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, KEYSTORE);
        generator.init(new KeyGenParameterSpec.Builder(KEY_ALIAS,
                KeyProperties.PURPOSE_ENCRYPT | KeyProperties.PURPOSE_DECRYPT)
                .setBlockModes(KeyProperties.BLOCK_MODE_GCM)
                .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE)
                .setKeySize(256)
                .build());
        return generator.generateKey();
    }
}
