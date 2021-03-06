// Disable requiring FunctionExpression JSDoc, because this file
// uses many function expressions as lambda expressions where
// it is overkill to require documentation.
/*
eslint "require-jsdoc": ["error", {
    "require": {
        "FunctionExpression": false
    }
}]
*/
const path = require('path');
const fs = require('fs');

const inquirer = require('inquirer');
const consts = require('./constants');
const Provider = require('./provider');


/**
  * Return a list of all provider names as listed in providers.json.
  *
  * @returns {string[]} A list of provider names.
  */
function allProviders() {
  const providerFile = path.join(__dirname, 'providers.json');
  const providerInfo = JSON.parse(fs.readFileSync(providerFile, 'utf8'));
  return Object.keys(providerInfo);
}

/**
  * Throw an error if the input string does not contain a number.
  *
  * @param {string} input The string to check.
  * @returns {void}
  */
function isNumber(input) {
  if (!(/^\d+$/.test(input))) {
    throw new Error('Please provide a number');
  }
  return true;
}

/**
  * Converts a map from friendly names to formal values into an array of objects
  * compatible with the `choices` attribute of inquirer questions. For example,
  * `{small: 't2.micro'}` would convert to
  * `[{name: 'small (t2.micro)', value: 't2.micro'}].
  *
  * @param {Object.<string, string>} friendlyNameToValue - The data to make descriptions for.
  * @returns {Object.<string, string>[]} An array of {name, val} objects.
  *
  */
function getInquirerDescriptions(friendlyNameToValue) {
  return Object.keys(friendlyNameToValue).map((friendlyName) => {
    const formalValue = friendlyNameToValue[friendlyName];
    return { name: `${friendlyName} (${formalValue})`, value: formalValue };
  });
}

/**
 * @param {Object} question - An object with fields representing a question to
 *   ask a user.
 * @param {string} helpstring - The helpstring to show a user if they type '?' in
 *   response to the question.
 * @returns {Object} The given question, with a modified `message` that now suggests
 *   using '?', and a modified `validate` function that will print the helpstring if
 *   the user enters '?'.
 */
function questionWithHelp(question, helpstring) {
  const newQuestion = Object.assign({}, question);

  newQuestion.message = `(type ? for help) ${question.message}`;
  newQuestion.validate = function validate(input) {
    if (input === '?') {
      return helpstring;
    }
    if (question.validate === undefined) {
      return true;
    }
    return question.validate(input);
  };
  return newQuestion;
}

// User prompts.

/**
  * If a base infrastructure exists, ask the user if they want to overwrite it.
  *
  * @returns {Promise} A promise that contains the user's answers.
  */
function overwritePrompt() {
  const questions = [{
    type: 'confirm',
    name: consts.infraOverwrite,
    message: 'This will overwrite your existing base infrastructure. Continue?',
    default: false,
    when() {
      return fs.existsSync(consts.baseInfraLocation);
    },
  },
  ];
  return inquirer.prompt(questions);
}

/**
  * Prompt the user for for their desired provider.
  *
  * @returns {Promise} A promise that contains the user's answer.
  */
function getProviderPrompt() {
  const questions = [
    {
      type: 'list',
      name: consts.provider,
      message: 'Choose a provider:',
      choices() {
        return allProviders();
      },
    },
  ];

  return inquirer.prompt(questions);
}

/**
  * Ask the user about provider credentials.
  *
  * @param {Provider} provider The provider to set up credentials for.
  * @returns {Promise} A promise that contains the user's answers.
  */
function credentialsPrompts(provider) {
  if (!provider.requiresCreds()) {
    return new Promise((resolve) => {
      resolve({});
    });
  }

  const keyExists = provider.credsExist();
  const questions = [
    {
      type: 'confirm',
      name: consts.providerUseExistingKey,
      message: `Use existing keys for ${provider.getName()}` +
        ` (${provider.getCredsPath()})?`,
      default: true,
      when() {
        return keyExists;
      },
    },
    {
      type: 'confirm',
      name: consts.credsConfirmOverwrite,
      message: 'This will overwrite the existing credentials. Continue?' +
        ' (If no, use existing credentials)',
      default: false,
      when(answers) {
        return answers[consts.providerUseExistingKey] === false;
      },
    },
  ];

  const keys = provider.getCredsKeys();
  const keyNames = Object.keys(keys);

  const credsHelp = `Kelda needs access to your ${provider.getName()} ` +
  'credentials in order to launch VMs in your account. See details at ' +
  'http://docs.kelda.io/#cloud-provider-configuration';

  keyNames.forEach((keyName) => {
    questions.push(
      questionWithHelp({
        type: 'input',
        name: keyName,
        message: `${keys[keyName]}:`,
        // Ask this questions for all credential inputs that are not paths.
        // I.e. keys given exactly as they should appear in the credential file.
        when(answers) {
          return (keyName !== consts.inputCredsPath &&
            (!keyExists || answers[consts.credsConfirmOverwrite]));
        },
        filter(input) {
          return input.trim();
        },
      }, credsHelp),
      questionWithHelp({
        type: 'input',
        name: keyName,
        message: `${keys[keyName]}:`,

        // Ask this question if the credentials should be given as a file path.
        // E.g. the path to the GCE project ID file.
        when(answers) {
          return keyName === consts.inputCredsPath &&
            (!keyExists || answers[consts.credsConfirmOverwrite]);
        },

        validate(input) {
          if (fs.existsSync(input)) return true;
          return `Oops, no file called "${input}".`;
        },

        filter(input) {
          return path.resolve(input);
        },
      }, credsHelp));
  });
  return inquirer.prompt(questions);
}

/**
  * Ask the user for machine configuration, such as size and region.
  *
  * @param {Provider} provider The provider chosen for this infrastructure.
  * @returns {Promise} A promise that contains the user's answers.
  */
function machineConfigPrompts(provider) {
  const regionChoices = getInquirerDescriptions(provider.getRegions());
  const sizeChoices = getInquirerDescriptions(provider.getSizes());
  sizeChoices.push({ name: consts.other, value: consts.other });

  const questions = [
    {
      type: 'confirm',
      name: consts.preemptible,
      message: 'Do you want to run preemptible instances?',
      default: false,
      when() {
        return provider.hasPreemptible;
      },
    },
    {
      type: 'list',
      name: consts.region,
      message: 'Which region do you want to deploy in?',
      choices: regionChoices,
      when() {
        return provider.regions !== undefined;
      },
    },
    {
      type: 'list',
      name: consts.size,
      message: 'What machine size do you want?',
      choices: sizeChoices,
      when() {
        return provider.sizes !== undefined;
      },
    },
    {
      type: 'input',
      name: consts.size,
      message: 'Which other instance type?',
      validate(input) {
        if (input !== '') return true;
        return 'Please provide an instance type';
      },
      when(answers) {
        return answers[consts.size] === consts.other;
      },
    },
    questionWithHelp({
      type: 'input',
      name: consts.cpu,
      message: 'How many CPUs do you want?',
      default: 1,
      validate(input) {
        return isNumber(input);
      },
      when() {
        return provider.sizes === undefined;
      },
    }, 'For small applications, 1 CPU is probably enough.'),
    questionWithHelp({
      type: 'input',
      name: 'ram',
      message: 'How many GiB of RAM do you want?',
      default: 2,
      validate(input) {
        return isNumber(input);
      },
      when() {
        return provider.sizes === undefined;
      },
    }, 'For small applications, 2 GiB is a suitable choice.'),
  ];

  return inquirer.prompt(questions);
}

/**
  * Ask for the desired number of machines.
  *
  * @returns {Promise} A promise that contains the user's answers.
  */
function machineCountPrompts() {
  const questions = [
    questionWithHelp({
      type: 'input',
      name: consts.masterCount,
      message: 'How many master VMs?',
      default: 1,
      validate(input) {
        return isNumber(input);
      },
    }, 'Master VMs are in charge of keeping your application running. Most ' +
    'users just need 1, but more can be added for fault tolerance.'),
    questionWithHelp({
      type: 'input',
      name: consts.workerCount,
      message: 'How many worker VMs?',
      default: 1,
      validate(input) {
        return isNumber(input);
      },
    }, 'Worker VMs run your application code. For small applications, 1 ' +
    'worker is typically enough.'),
  ];

  return inquirer.prompt(questions);
}

/**
 * Prompt the user to get the information needed to create the new
 * infrastructure.
 *
 * @returns {Promise} A promise that contains the user's answers.
 */
function promptUser() {
  const answers = {};
  return overwritePrompt()
    .then((shouldOverwrite) => {
      if (shouldOverwrite[consts.infraOverwrite] === false) {
        return { shouldAbort: true };
      }
      return getProviderPrompt()
        .then((providerAnswer) => {
          Object.assign(answers, providerAnswer);
          const provider = new Provider(answers[consts.provider]);
          return credentialsPrompts(provider)

            .then((keyAnswers) => {
              Object.assign(answers, keyAnswers);
              return machineConfigPrompts(provider);
            })

            .then((providerAnswers) => {
              Object.assign(answers, providerAnswers);
              return machineCountPrompts();
            })

            .then((machineAnswers) => {
              Object.assign(answers, machineAnswers);
              return { provider, answers };
            });
        });
    });
}

module.exports = { promptUser };
