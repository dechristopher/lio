import React, { FC } from "react";
import styles from "./Clock.module.scss";
import classNames from "classnames";

interface ClockProps {
	time: string;
	score: number;
	isWhite: boolean;
	isActive: boolean;
	barWidth: number;
	flipOrientation?: boolean;
}

const Clock: FC<ClockProps> = (props) => {
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
					[styles.active]: props.isActive,
					[styles.flip]: !props.flipOrientation,
				})}
			>
				<div className={styles.clockNameContainer}>
					<div className={styles.clockScore}>0</div>
					<div
						className={classNames({
							[styles.clockName]: true,
							[styles.playerWhite]: props.isWhite,
							[styles.playerBlack]: !props.isWhite,
						})}
					>
						Opponent
					</div>
					<div className={styles.clockRatingNumber}>
						{props.score}
					</div>
				</div>

				<div
					className={classNames({
						[styles.clockTime]: true,
						[styles.active]: props.isActive,
					})}
				>
					{props.time}
				</div>
			</div>

			<div className={styles.clockProgress}>
				<div
					className={classNames({
						[styles.clockProgressBar]: true,
						[styles.active]: props.isActive,
					})}
					style={{
						width: `${props.barWidth}%`,
					}}
				></div>
				<div className={styles.clockProgressBg}></div>
			</div>
		</div>
	);
};

export default Clock;
