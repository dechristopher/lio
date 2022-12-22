"use client";

import React, { FC, useMemo, useState } from "react";
import styles from "./Clock.module.scss";
import classNames from "classnames";
import { useAnimationFrame } from "@client/hooks/useAnimationFrame";
import { PlayerColor } from "@client/proto/ws_pb";
import dayjs from "dayjs";
import duration, { Duration } from "dayjs/plugin/duration";
dayjs.extend(duration);

export type ClockState = {
	score: number;
	isPlayerTurn: boolean;
	gameStarted: boolean;
	initialTime: Duration;
	timeRemaining: Duration;
	playerColor: PlayerColor.WHITE | PlayerColor.BLACK;
};

export interface ClockProps {
	state: ClockState;
	flipOrientation: boolean;
	setIsActive: (isActive: boolean) => void;
}

const Clock: FC<ClockProps> = (props) => {
	const [barWidth, setBarWidth] = useState<number>(100);
	const [time, setTime] = useState<string>(
		formatDuration(props.state.initialTime),
	);
	const animateClock = useMemo(() => {
		if (!props.state.gameStarted) {
			setBarWidth(100);
			setTime(formatDuration(props.state.initialTime));
		}

		return props.state.isPlayerTurn && props.state.gameStarted;
	}, [
		props.state.gameStarted,
		props.state.initialTime,
		props.state.isPlayerTurn,
	]);

	const clockAnimationFrameHandler = (
		frameTime: number, // when the clock started to animate in ms
	) => {
		const elapsedTime = performance.now() - frameTime;
		const remainingTime = props.state.timeRemaining.subtract(
			elapsedTime,
			"milliseconds",
		);

		if (remainingTime.asMilliseconds() < 0) {
			props.setIsActive(false);
		} else {
			setTime(formatDuration(remainingTime));
			setBarWidth(calcBarWidth(props.state.initialTime, remainingTime));
		}
	};

	useAnimationFrame(clockAnimationFrameHandler, animateClock);

	return (
		<div
			className={classNames({
				[styles.container]: true,
				[styles.flip]: !props.flipOrientation,
			})}
		>
			<div
				className={classNames({
					[styles.clockContainer]: true,
					[styles.active]: animateClock,
					[styles.flip]: !props.flipOrientation,
				})}
			>
				<div className={styles.clockNameContainer}>
					<div className={styles.clockScore}>0</div>
					<div
						className={classNames({
							[styles.clockName]: true,
							[styles.playerWhite]:
								props.state.playerColor === PlayerColor.WHITE,
							[styles.playerBlack]:
								props.state.playerColor === PlayerColor.BLACK,
						})}
					>
						Opponent
					</div>
					<div className={styles.clockRatingNumber}>
						{props.state.score}
					</div>
				</div>

				<div
					className={classNames({
						[styles.clockTime]: true,
						[styles.active]: animateClock,
					})}
				>
					{time}
				</div>
			</div>

			<div className={styles.clockProgress}>
				<div
					className={classNames({
						[styles.clockProgressBar]: true,
						[styles.active]: animateClock,
					})}
					style={{
						width: `${barWidth}%`,
					}}
				></div>
				<div className={styles.clockProgressBg}></div>
			</div>
		</div>
	);
};

/**
 * Returns a CSS width percentage based on the percentage of
 * the clock time remaining for the given time control
 * @param timeControl - time control total centiseconds
 * @param time - centiseconds remaining
 * @returns {`${number}%`}
 */
const calcBarWidth = (timeControl: Duration, time: Duration): number => {
	return Math.min(
		(time.asMilliseconds() / timeControl.asMilliseconds()) * 100,
		100,
	);
};

function formatDuration(duration: Duration): string {
	const durationStr = duration.format("mm:ss:SSS");
	// show to the decisecond when time >= 10
	// show to the centisecond when time < 10
	const sliceEnd = duration.asSeconds() < 10 ? 1 : 2;
	return durationStr.slice(0, durationStr.length - sliceEnd);
}

export default Clock;
