package com.vanillapay.monitor

import android.content.Context
import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import android.util.Base64
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import kotlinx.coroutines.flow.first
import org.json.JSONObject
import java.security.KeyStore
import javax.crypto.Cipher
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey
import javax.crypto.spec.GCMParameterSpec

private val Context.vault by preferencesDataStore("monitor_vault")
private val entry = stringPreferencesKey("sealed_credential")
private const val alias = "vanillapay-monitor-credentials-v1"

class CredentialVault(private val context: Context) {
    suspend fun load(): Credential? {
        val encoded = context.vault.data.first()[entry] ?: return null
        return try {
            val data = Base64.decode(encoded, Base64.NO_WRAP)
            val cipher = Cipher.getInstance("AES/GCM/NoPadding")
            cipher.init(Cipher.DECRYPT_MODE, key(create = false), GCMParameterSpec(128, data.copyOfRange(0, 12)))
            val json = JSONObject(String(cipher.doFinal(data, 12, data.size - 12), Charsets.UTF_8))
            val base = json.getString("base")
            require(siteUrl(base) != null)
            Credential(base, json.getString("id"), json.getString("token"), json.getString("name"),
                json.getLong("bound_at"), json.getInt("interval"), json.getLong("offset"), json.getBoolean("started"))
        } catch (_: java.security.GeneralSecurityException) {
            clear()
            null
        } catch (_: org.json.JSONException) {
            clear()
            null
        } catch (_: IllegalArgumentException) {
            clear()
            null
        }
    }

    suspend fun save(credential: Credential) {
        val plain = JSONObject().put("base", credential.base).put("id", credential.deviceId).put("token", credential.token)
            .put("name", credential.deviceName).put("bound_at", credential.boundAt).put("interval", credential.interval)
            .put("offset", credential.offsetMillis).put("started", credential.started).toString()
        val cipher = Cipher.getInstance("AES/GCM/NoPadding")
        cipher.init(Cipher.ENCRYPT_MODE, key(create = true))
        val sealed = Base64.encodeToString(cipher.iv + cipher.doFinal(plain.toByteArray(Charsets.UTF_8)), Base64.NO_WRAP)
        context.vault.edit { it[entry] = sealed }
    }

    suspend fun clear() {
        context.vault.edit { it.remove(entry) }
    }

    private fun key(create: Boolean): SecretKey {
        val store = KeyStore.getInstance("AndroidKeyStore").apply { load(null) }
        (store.getKey(alias, null) as? SecretKey)?.let { return it }
        if (!create) throw java.security.InvalidKeyException("Credential key is unavailable")
        val spec = KeyGenParameterSpec.Builder(alias, KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT)
            .setKeySize(256)
            .setBlockModes(KeyProperties.BLOCK_MODE_GCM)
            .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE)
            .build()
        return KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, "AndroidKeyStore").run {
            init(spec)
            generateKey()
        }
    }
}
