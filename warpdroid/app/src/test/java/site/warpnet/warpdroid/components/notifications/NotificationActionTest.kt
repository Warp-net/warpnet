/*
 * Warpdroid - a Warpnet Android client.
 * Copyright (C) 2026 Warpdroid contributors.
 *
 * This program is free software: you can redistribute it and/or modify it
 * under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or (at
 * your option) any later version.
 */
package site.warpnet.warpdroid.components.notifications

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class NotificationActionTest {
    @Test
    fun `the action stays whatever name the node stored`() {
        assertEquals(" started following you", notificationAction("Anonymous started following you"))
        assertEquals(" started following you", notificationAction("Bob Smith started following you"))
    }

    @Test
    fun `every action the node writes is recognised`() {
        listOf(
            "started following you",
            "reacted your tweet",
            "retweeted your tweet",
            "quoted your tweet",
            "replied to your tweet",
            "sent you a message",
            "joined Warpnet",
        ).forEach { action -> assertEquals(" $action", notificationAction("bob $action")) }
        assertEquals(
            " bought 10 of your tweets within an hour; their next orders wait",
            notificationAction("leaker bought 10 of your tweets within an hour; their next orders wait"),
        )
    }

    @Test
    fun `an unknown text has no action`() {
        assertNull(notificationAction("your report was reviewed"))
        assertNull(notificationAction("started following you"))
    }
}
