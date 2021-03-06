import React, {FC} from 'react';
import {BackGroundColors, BorderColors, TextColors} from '@utils/styles/colors'
import classNames from "classnames";

interface GameVariantCardProps {
    width: number;
    height: number;
    bgColor: BackGroundColors;
    textColor: TextColors;
    borderColor?: BorderColors;
    centerContent?: boolean;
    borderRadius?: number;
}

export const GameVariantCard: FC<GameVariantCardProps> = (props) => {
    const {width, height, bgColor, textColor, borderColor, children} = props

    return (
        <div
            className={classNames(
                bgColor,
                textColor,
                borderColor,
                {
                    "flex justify-center items-center": props.centerContent
                }
            )}
            style={{width, height, borderRadius: props.borderRadius || 5 }}
        >
            {children}
        </div>
    )
}

