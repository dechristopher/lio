import React, {FC} from 'react'
import {FontSizes, FontWeights} from "@utils/constants";
import classNames from "classnames";
import {textColors, TextColors} from "@utils/styles/colors";

interface GameVariantTextProps {
    timeControl: string;
    variantName?: string;
    nameOnSameLine?: boolean;
    className?: string;
    timeControlFontSize?: FontSizes;
    variantNameFontSize?: FontSizes;
    timeControlFontWeight?: FontWeights
    variantNameFontWeight?: FontWeights;
    textColor?: TextColors;
}

/**
 * Text component for displaying a given game variation.
 *
 * @param {GameVariantTextProps} props - game variant text props
 * @param {string} props.timeControl - game time control
 * @param {string} props.variantName - game variant name
 * @param {FontWeights} props.timeControlFontWeights - time control font weight
 * @param {FontWeights} props.variantNameFontWeights - variant name font weight
 *
 * @returns {Element} - game variant text component
 *
 * @example
 * <GameVariantText
 *   timeControl="1 + 0"
 *   variantName="Rapid"
 *   timeControlFontWeight={FontWeights.medium}
 *   variantNameFontWeight={FontWeights.semibold}
 * />
 */
export const GameVariantText: FC<GameVariantTextProps> = (props) => {
    return (
        <div className={classNames(
            "flex flex-col text-center md:p-4",
            props.className
        )}>
            <span className={
                classNames(
                    props.timeControlFontSize,
                    props.timeControlFontWeight,
                    props.textColor
                )}
            >
                {props.nameOnSameLine && props.variantName ?
                    `${props.timeControl} ${props.variantName}` :
                    props.timeControl}
            </span>
            {props.variantName && !props.nameOnSameLine ?
                <span className={
                    classNames(
                        props.variantNameFontSize,
                        props.variantNameFontWeight,
                        props.textColor
                    )}>
                    {props.variantName}
                </span> : null}
        </div>
    )
}

GameVariantText.defaultProps = {
    textColor: textColors.black["1000"],
    timeControlFontSize: FontSizes.FourXL,
    variantNameFontSize: FontSizes.TwoXL,
    timeControlFontWeight: FontWeights.SemiBold,
    variantNameFontWeight: FontWeights.SemiBold
}