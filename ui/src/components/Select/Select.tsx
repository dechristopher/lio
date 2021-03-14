import React, {FC, useEffect, useState} from 'react';
import classNames from "classnames";

interface SelectProps {
    label: string;
    selectOptions: (string | number)[],
    defaultValue?: string | number;
    className?: string;
    onSelect?: (value: string | number) => void;
}

export const Select: FC<SelectProps> = (props) => {
    const [currentValue, setCurrentValue] = useState<string | number | undefined>(props.defaultValue)
    const [showOptions, setShowOptions] = useState<boolean>(false)

    /**
     * If the current value is undefined, set it to the first option
     */
    useEffect(() => {
        if (!currentValue) {
            setCurrentValue(props.selectOptions[0])
        }
    }, [])

    return (
        <div className={classNames(
            {
                [props.className as string]: true
            }
        )}>
            <label id="listbox-label" className={classNames(
                "block text-sm font-medium text-gray-700"
            )}>
                {props.label}
            </label>
            <div
                className="mt-1 relative"
            >
                <button
                    type="button"
                    aria-haspopup="listbox"
                    aria-expanded="true"
                    aria-labelledby="listbox-label"
                    onClick={() => setShowOptions(!showOptions)}
                    onBlur={(e) => {
                        // if a related target exists on the event (i.e user clicked on a selection
                        // option, don't close the menu
                        if (!e.relatedTarget) {
                            setShowOptions(false)
                        }
                    }}
                    style={{minWidth: "8rem"}}
                    className="relative w-full bg-white border border-gray-300 rounded-md shadow-sm pl-3 pr-10 py-2 text-left cursor-default focus:outline-none focus:ring-1 focus:ring-indigo-500 focus:border-indigo-500 sm:text-sm">
                    <span className="block truncate">
                        {currentValue}
                    </span>

                    {/* Select arrows */}
                    <span className="absolute inset-y-0 right-0 flex items-center pr-2 pointer-events-none">
                        <svg className="h-5 w-5 text-gray-400" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20"
                             fill="currentColor" aria-hidden="true">
                          <path fillRule="evenodd"
                                d="M10 3a1 1 0 01.707.293l3 3a1 1 0 01-1.414 1.414L10 5.414 7.707 7.707a1 1 0 01-1.414-1.414l3-3A1 1 0 0110 3zm-3.707 9.293a1 1 0 011.414 0L10 14.586l2.293-2.293a1 1 0 011.414 1.414l-3 3a1 1 0 01-1.414 0l-3-3a1 1 0 010-1.414z"
                                clipRule="evenodd"/>
                        </svg>
                    </span>
                </button>

                {/* Select options */}
                <div
                    className={classNames(
                        "absolute",
                        "mt-1",
                        "w-full",
                        "rounded-md",
                        "bg-white",
                        "shadow-lg",
                        {
                            "select-options-hidden": !showOptions,
                            "select-options-visible": showOptions
                        },
                    )}
                    style={{
                        zIndex: 1,
                    }}
                >
                    <ul
                        tabIndex={-1}
                        role="listbox"
                        aria-labelledby="listbox-label"
                        className="max-h-60 rounded-md py-1 text-base ring-1 ring-black ring-opacity-5 overflow-auto focus:outline-none sm:text-sm"
                    >
                        {props.selectOptions.map((opt, key) => {
                            return (
                                <li
                                    key={key}
                                    id={`listbox-item-${key}`}
                                    role="option"
                                    className={classNames(
                                        "text-gray-900 cursor-default select-none relative py-2 pl-8 pr-4 hover:bg-green-300",
                                    )}
                                    aria-selected={opt === currentValue}
                                    onClick={() => {
                                        setCurrentValue(opt)

                                        // send clicked value
                                        if (props.onSelect) {
                                            props.onSelect(opt)
                                        }

                                        setTimeout(() => {
                                            setShowOptions(false)
                                        }, 0)
                                    }}
                                >
                                    <span className="font-normal block truncate">
                                        {opt}
                                    </span>

                                    {/* Check mark */}
                                    {opt === currentValue ?
                                        <span className="absolute inset-y-0 left-0 flex items-center pl-1.5">
                                            <svg
                                           className="h-5 w-5"
                                           xmlns="http://www.w3.org/2000/svg"
                                           viewBox="0 0 20 20"
                                           fill="currentColor"
                                           aria-hidden="true"
                                            >
                                                <path
                                                    fillRule="evenodd"
                                                    d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z"
                                                    clipRule="evenodd"
                                                />
                                            </svg>
                                        </span> : null}
                                </li>
                            )
                        })}
                    </ul>
                </div>
            </div>
        </div>
    );
};

Select.defaultProps = {
    className: ""
}