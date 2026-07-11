package com.bench.shared.db

import com.datastax.oss.driver.api.core.addresstranslation.AddressTranslator
import com.datastax.oss.driver.api.core.config.DefaultDriverOption
import com.datastax.oss.driver.api.core.context.DriverContext
import java.net.InetSocketAddress

/**
 * Redirects every peer address the cluster advertises to the reachable contact
 * point. The benchmark Cassandra is single-node and its compose config sets
 * `broadcast_rpc_address = 127.0.0.1` (so host clients on the published port work),
 * so the driver would otherwise open its per-node pool to 127.0.0.1 — which, from
 * another container, is the client itself. Rewriting every discovered address to
 * the contact point fixes it (identical intent to `shared/rust`'s translator).
 *
 * Instantiated reflectively by the driver, so the constructor takes a
 * `DriverContext`; the target is read from the driver's own CONTACT_POINTS option
 * (no external state).
 */
class ContactPointTranslator(
    private val context: DriverContext,
) : AddressTranslator {
    private val target: InetSocketAddress by lazy {
        val contactPoint =
            context.config.defaultProfile
                .getStringList(DefaultDriverOption.CONTACT_POINTS)
                .first()
        val parts = contactPoint.split(":", limit = 2)
        InetSocketAddress(parts[0], parts.getOrNull(1)?.toIntOrNull() ?: DEFAULT_PORT)
    }

    override fun translate(address: InetSocketAddress): InetSocketAddress = target

    override fun close() = Unit

    private companion object {
        const val DEFAULT_PORT = 9042
    }
}
