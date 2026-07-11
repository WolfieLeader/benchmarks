package com.bench.shared.web

import com.auth0.jwt.JWT
import com.auth0.jwt.algorithms.Algorithm
import com.auth0.jwt.exceptions.JWTVerificationException
import com.bench.shared.Consts
import kotlinx.serialization.Serializable
import java.time.Instant
import java.util.Date

/**
 * HS256 JWT sign/verify for the web suite, via auth0 `java-jwt` (a popular
 * production library over hand-rolled base64/HMAC — CLAUDE.md standing decision;
 * kotlin.md item 55 blesses java-jwt for the classic-JWT case). Shared by every
 * Kotlin server so both lanes sign/verify identical tokens.
 */
object Jwt {
    /** Sign a token with the fixed contract claims plus a fresh iat/exp (1h TTL). */
    fun sign(secret: String): String {
        val now = Instant.now()
        return JWT
            .create()
            .withSubject(Consts.JWT_SUBJECT)
            .withClaim("name", Consts.JWT_NAME)
            .withClaim("admin", Consts.JWT_ADMIN)
            .withIssuedAt(Date.from(now))
            .withExpiresAt(Date.from(now.plusSeconds(Consts.JWT_TTL_SECONDS)))
            .sign(Algorithm.HMAC256(secret))
    }

    /**
     * Verify signature + expiry and return the decoded claims, or `null` for a
     * malformed/wrong-signature/expired token. The java-jwt exception is caught
     * here so it never crosses the shared-module boundary (the driver stays an
     * internal `implementation` dep); the caller maps null to
     * `401 {"error":"invalid token"}`.
     */
    fun verify(
        secret: String,
        token: String,
    ): JwtClaims? =
        try {
            val decoded = JWT.require(Algorithm.HMAC256(secret)).build().verify(token)
            JwtClaims(
                sub = decoded.subject,
                name = decoded.getClaim("name").asString(),
                admin = decoded.getClaim("admin").asBoolean(),
                iat = decoded.issuedAt.toInstant().epochSecond,
                exp = decoded.expiresAt.toInstant().epochSecond,
            )
        } catch (_: JWTVerificationException) {
            null
        }
}

/** The `/jwt/verify` response payload — echoed exactly (iat/exp as JSON numbers). */
@Serializable
data class JwtClaims(
    val sub: String,
    val name: String,
    val admin: Boolean,
    val iat: Long,
    val exp: Long,
)
