import React, {FC} from "react";
import classNames from "classnames";

import {BGColor} from "@utils/styles/colors";

export interface ProgressBarProps {
	progress: number; // decimal progress percentage (0 <= progress <= 1),
	color: BGColor;
}

export const ProgressBar: FC<ProgressBarProps> = ({
	                                                  progress = 0,
	                                                  color = new BGColor("green")
                                                  }) => {
	return (
		<div className="progress-bar">
			<div
				style={{width: `${Math.round(progress * 100)}%`}}
				className={classNames("progress-bar__inner", {
					[color.getColor()]: !!color.getColor()
				})}
			/>
		</div>
	)
}