/* Copyright 2025 Tusky Contributors
 *
 * This file is a part of Tusky.
 *
 * This program is free software; you can redistribute it and/or modify it under the terms of the
 * GNU General Public License as published by the Free Software Foundation; either version 3 of the
 * License, or (at your option) any later version.
 *
 * Tusky is distributed in the hope that it will be useful, but WITHOUT ANY WARRANTY; without even
 * the implied warranty of MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the GNU General
 * Public License for more details.
 *
 * You should have received a copy of the GNU General Public License along with Tusky; if not,
 * see <http://www.gnu.org/licenses>. */

package com.keylesspalace.tusky.util.savedstateflow

import androidx.lifecycle.SavedStateHandle
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.ExperimentalForInheritanceCoroutinesApi
import kotlinx.coroutines.flow.FlowCollector
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow

/**
 * A wrapper around [MutableStateFlow] that automatically saves the state in a [SavedStateHandle].
 */
@OptIn(ExperimentalForInheritanceCoroutinesApi::class)
internal class SavedStateFlow<T>(
    private val savedStateHandle: SavedStateHandle,
    private val key: String,
    private val initialValue: T
) : MutableStateFlow<T> {

    private val state: MutableStateFlow<T> =
        MutableStateFlow(savedStateHandle[key] ?: initialValue)

    override var value: T
        get() = state.value
        set(value) {
            savedStateHandle[key] = value
            state.value = value
        }

    override fun compareAndSet(expect: T, update: T): Boolean {
        val result = state.compareAndSet(expect, update)
        if (result) {
            savedStateHandle[key] = update
        }
        return result
    }

    fun asStateFlow(): StateFlow<T> = state.asStateFlow()

    override val replayCache: List<T>
        get() = state.replayCache

    override suspend fun collect(collector: FlowCollector<T>): Nothing {
        state.collect(collector)
    }

    override val subscriptionCount: StateFlow<Int>
        get() = state.subscriptionCount

    override suspend fun emit(value: T) {
        state.emit(value)
    }

    override fun tryEmit(value: T): Boolean {
        return state.tryEmit(value)
    }

    @ExperimentalCoroutinesApi
    override fun resetReplayCache() {
        state.resetReplayCache()
    }

    fun isChanged(): Boolean {
        return state.value != initialValue
    }
}
