package com.bench.spring.web

import com.bench.shared.Consts
import com.bench.shared.Env
import com.bench.shared.model.OrderValidation
import com.bench.shared.model.ValidateRequest
import com.bench.shared.web.Compute
import com.bench.shared.web.Jwt
import com.bench.shared.web.JwtClaims
import com.bench.spring.appJson
import com.bench.spring.error.ApiException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import kotlinx.serialization.SerializationException
import org.springframework.http.HttpHeaders
import org.springframework.web.bind.annotation.GetMapping
import org.springframework.web.bind.annotation.PostMapping
import org.springframework.web.bind.annotation.RequestHeader
import org.springframework.web.bind.annotation.RequestParam
import org.springframework.web.bind.annotation.RestController
import org.springframework.web.server.ServerWebExchange

/** The web suite: `GET /jwt/sign`, `GET /jwt/verify`, `POST /validate`, `GET /compute`. */
@RestController
class WebController(
    private val env: Env,
) {
    @GetMapping("/jwt/sign")
    fun sign(): TokenResponse = TokenResponse(Jwt.sign(env.jwtSecret))

    @GetMapping("/jwt/verify")
    fun verify(
        @RequestHeader(name = HttpHeaders.AUTHORIZATION, required = false) authorization: String?,
    ): JwtClaims {
        if (authorization == null || !authorization.startsWith(BEARER_PREFIX)) {
            throw ApiException.Unauthorized(Consts.ERR_INVALID_TOKEN)
        }
        return Jwt.verify(env.jwtSecret, authorization.removePrefix(BEARER_PREFIX))
            ?: throw ApiException.Unauthorized(Consts.ERR_INVALID_TOKEN)
    }

    @PostMapping("/validate")
    suspend fun validate(exchange: ServerWebExchange): ValidResponse {
        // Decode failures are malformed JSON / type mismatches only (every schema field
        // has a zero-value default, Go canon) — so they map to the invalid-JSON body
        // error; rule breaches are the validator's job below.
        val request =
            try {
                appJson.decodeFromString<ValidateRequest>(readBody(exchange))
            } catch (e: SerializationException) {
                throw ApiException.BadRequest(Consts.ERR_INVALID_JSON, e.message ?: "invalid payload", e)
            }
        val errors = OrderValidation.validate(request)
        if (errors.isNotEmpty()) {
            throw ApiException.BadRequest(Consts.ERR_VALIDATION_FAILED, errors.joinToString("; "))
        }
        return ValidResponse(true)
    }

    @GetMapping("/compute")
    suspend fun compute(
        @RequestParam(required = false) n: String?,
    ): ComputeResponse {
        // Strict integer parse: toLongOrNull rejects underscores/decimals/non-numerics.
        val rounds = n?.toLongOrNull()
        if (rounds == null || rounds < 1) {
            throw ApiException.BadRequest(Consts.ERR_INVALID_N, "n must be an integer >= 1")
        }
        // CPU-bound chain off the Netty event loop (item 58) — same algorithm as kt-ktor.
        val result = withContext(Dispatchers.Default) { Compute.sha256Chain(Compute.clampRounds(rounds)) }
        return ComputeResponse(result)
    }

    private companion object {
        const val BEARER_PREFIX = "Bearer "
    }
}
